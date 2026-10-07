package processor

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"go.uber.org/zap"
	"gorm.io/datatypes"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

const (
	MaxErrorsToDisplay = 100 // 最多展示的错误数量
	MaxErrorsToCollect = 100 // 最多收集的错误数量
	SampleErrorCount   = 5   // 出错时打印的错误样本数量
)

// getOptimalConfig 根据机器 CPU 核数返回各类缓冲区与批量大小的推荐值。
// 核数越多配置越激进：2 核（CI 环境）保守，4-8 核折中，10 核以上放开。
func getOptimalConfig() (workBuffer, resultBuffer, errorBuffer, defaultBatch, minBatch, maxBatch int) {
	cpuCount := runtime.NumCPU()

	switch {
	case cpuCount <= 2:
		// GitHub Actions 等低配 CI
		return 50, 1000, 50, 200, 50, 300

	case cpuCount <= 4:
		// 入门级机器
		return 75, 2000, 75, 300, 100, 500

	case cpuCount <= 8:
		// 中端机器
		return 100, 3000, 100, 400, 150, 700

	default:
		// 高端机器
		return 500, 10000, 500, 1000, 500, 2000
	}
}

// Processor 负责并发处理诗词数据。
type Processor struct {
	repo                 database.RepositoryInterface
	workers              int
	convertToTraditional bool
	batchSize            int // 写入数据库时的批量大小

	// authorIDs 是 prewarmCache 规划好的作者 ID，键为 authorIdentity。
	// processPoem 按身份而非转换后的名字取 ID：同一位诗人在不同来源中写法不同，
	// 转换后的名字可能对不上（见 planAuthors）。
	authorIDs map[string]int64

	// bios 是作者文件中的小传，导入时挂到对应作者上，见 SetAuthorBios。
	bios []loader.AuthorBio
}

// SetAuthorBios 设置要随作者一起导入的诗人小传。
func (p *Processor) SetAuthorBios(bios []loader.AuthorBio) {
	p.bios = bios
}

// NewProcessor 创建带缓存能力的处理器，workers <= 0 时按 CPU 核数取值。
func NewProcessor(repo *database.Repository, workers int, convertToTraditional bool) *Processor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	_, _, _, defaultBatch, _, _ := getOptimalConfig()

	// 包一层缓存，避免重复查询朝代/作者
	cachedRepo := database.NewCachedRepository(repo)

	return &Processor{
		repo:                 cachedRepo,
		workers:              workers,
		convertToTraditional: convertToTraditional,
		batchSize:            defaultBatch,
	}
}

// SetBatchSize 设置写入数据库时的批量大小。
func (p *Processor) SetBatchSize(size int) {
	if size > 0 {
		p.batchSize = size
	}
}

// prewarmCache 预先把朝代、作者写入缓存。
// 若不预热，所有 worker 会在冷缓存下同时读写数据库，
// 在 SQLite 单写者模型下会造成锁竞争，表现为疑似死锁。
func (p *Processor) prewarmCache(poems []loader.PoemWithMeta, sourceTrad []bool) error {
	// 先收集去重后的朝代（数量很少，约 20 个）
	dynastySet := make(map[string]struct{})
	for _, poem := range poems {
		if poem.Dynasty != "" {
			dynasty := poem.Dynasty
			converted, err := p.convertText(dynasty, p.convertToTraditional)
			if err != nil {
				continue // 出错则跳过，留到正式处理阶段再报
			}
			dynastySet[converted] = struct{}{}
		}
	}

	// 串行预热朝代缓存，无并发风险
	for dynasty := range dynastySet {
		if _, err := p.repo.GetOrCreateDynasty(dynasty); err != nil {
			return fmt.Errorf("failed to pre-warm dynasty cache for %q: %w", dynasty, err)
		}
	}

	// 作者按 planAuthors 给出的显式 ID 创建，简繁两套表因此共用一套作者 ID。
	authors, err := p.planAuthors(poems, sourceTrad, p.bios)
	if err != nil {
		return err
	}
	p.authorIDs = make(map[string]int64, len(authors))
	descriptions := make(map[int64]string)
	for _, a := range authors {
		dynastyName, err := p.convertText(a.dynasty, p.convertToTraditional)
		if err != nil {
			return fmt.Errorf("failed to convert dynasty %q: %w", a.dynasty, err)
		}
		dynastyID, err := p.repo.GetOrCreateDynasty(dynastyName)
		if err != nil {
			return fmt.Errorf("failed to create dynasty %q: %w", dynastyName, err)
		}
		id, err := p.repo.CreateAuthorWithID(a.id, a.name, dynastyID)
		if err != nil {
			return fmt.Errorf("failed to create author %q: %w", a.name, err)
		}
		p.authorIDs[a.identity] = id
		if a.description != "" && id == a.id {
			descriptions[id] = a.description
		}
	}
	if err := p.repo.SetAuthorDescriptions(descriptions); err != nil {
		return err
	}

	logger.Info("Cache pre-warmed",
		zap.Int("dynasties", len(dynastySet)),
		zap.Int("authors", len(authors)),
	)

	return nil
}

// plannedAuthor 是导入前规划好的一位作者。
type plannedAuthor struct {
	id          int64
	identity    string // 见 authorIdentity
	name        string // 已转为本语言变体的作者名
	dynasty     string // 原始（未转换的）朝代名
	description string // 已转为本语言变体的小传，没有则为空
}

// planAuthors 为语料中的作者分配 ID，结果与语言变体无关。
//
// 此前作者在遍历 map 时逐个创建，ID 由自增列按插入顺序给出，而 map 的遍历顺序
// 每次都不同：简繁两套表里同一位诗人的 ID 几乎完全对不上，依赖 ID 的跨表关联
// （如 GraphQL 嵌套字段）全都会查到别人。现在作者按其首次出现的诗词顺序编号，
// 同一份语料在两个变体、每次导入中都得到相同的 ID。
//
// 作者的身份是「简体名 + 朝代」（authorIdentity）：
//   - 以简体名为准：源数据中宋词为简体、全唐诗为繁体，同一位诗人（苏轼／蘇軾）
//     要归为一人；而简转繁有歧义，宋词里的「陆游」会被转成「陸遊」，以繁体名为准
//     就成了两个人（岳飞→嶽飛、杨万里→楊萬裏 同理）。繁体表里取首次出现时的写法。
//   - 带上朝代：同名异人很常见（唐代张潮与清代《幽梦影》作者张潮，各朝代的
//     「佚名」），只按名字区分会把他们并成一人、朝代也只能取其一。源数据本身也是
//     每个朝代各有一份作者表与小传；五代入宋、两部分都收了的诗人（徐铉）因此
//     对应唐、宋各一条记录，各配各的小传，与源数据一致。
//
// bios 是作者文件中的小传，按同样的身份挂到作者上；同一身份有多段不同的小传时
// （全唐诗作者表里正文小传与「《全唐詩小傳》云」补遗各一条，宋词与全宋诗各一份）
// 按出现顺序合并。
func (p *Processor) planAuthors(poems []loader.PoemWithMeta, sourceTrad []bool, bios []loader.AuthorBio) ([]plannedAuthor, error) {
	var authors []*plannedAuthor
	byIdentity := make(map[string]*plannedAuthor)

	for i, poem := range poems {
		// 与 processPoem 的跳过规则保持一致，免得为只有被跳过作品的作者建出空记录
		paragraphs := classifier.NormalizeAndSplitParagraphs(poem.Paragraphs)
		if len(paragraphs) == 0 || classifier.IsPlaceholderContent(paragraphs) {
			continue
		}

		author := classifier.NormalizeText(poem.Author)
		if author == "" {
			author = "佚名"
		}
		identity, err := authorIdentity(author, poem.Dynasty)
		if err != nil {
			return nil, err
		}
		if byIdentity[identity] != nil {
			continue
		}

		name, err := p.toVariant(author, sourceTrad[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert author %q: %w", author, err)
		}
		a := &plannedAuthor{id: int64(len(authors) + 1), identity: identity, name: name, dynasty: poem.Dynasty}
		authors = append(authors, a)
		byIdentity[identity] = a
	}

	if err := p.attachBios(byIdentity, bios); err != nil {
		return nil, err
	}

	planned := make([]plannedAuthor, len(authors))
	for i, a := range authors {
		planned[i] = *a
	}
	return planned, nil
}

// attachBios 把小传按作者身份挂到规划好的作者上，转为本语言变体，并合并重复。
// 没有作品的作者不会因为有小传而被建出来。
func (p *Processor) attachBios(byIdentity map[string]*plannedAuthor, bios []loader.AuthorBio) error {
	seen := make(map[string]map[string]bool) // 身份 -> 已收录小传的简体规整文本
	for _, bio := range bios {
		name := classifier.NormalizeText(bio.Name)
		identity, err := authorIdentity(name, bio.Dynasty)
		if err != nil {
			return err
		}
		a := byIdentity[identity]
		if a == nil {
			continue
		}

		desc := strings.TrimSpace(bio.Description)
		simplified, err := classifier.ToSimplified(desc)
		if err != nil {
			return fmt.Errorf("failed to convert bio of %q: %w", name, err)
		}
		// 同一段小传可能以简繁两种字形各出现一次，按简体文本判重
		if seen[identity] == nil {
			seen[identity] = make(map[string]bool)
		}
		if seen[identity][simplified] {
			continue
		}
		seen[identity][simplified] = true

		trad, err := classifier.IsTraditional(desc)
		if err != nil {
			return fmt.Errorf("failed to detect script of bio of %q: %w", name, err)
		}
		converted, err := p.toVariant(desc, trad)
		if err != nil {
			return fmt.Errorf("failed to convert bio of %q: %w", name, err)
		}
		if a.description == "" {
			a.description = converted
		} else {
			a.description += "\n\n" + converted
		}
	}
	return nil
}

// authorIdentity 返回判定「是否同一位作者」所用的键：canonicalAuthorName 求得的名字，
// 加上原始朝代名。author 须已经过 NormalizeText，空名已替换为「佚名」。
func authorIdentity(author, dynasty string) (string, error) {
	name, err := canonicalAuthorName(author)
	if err != nil {
		return "", err
	}
	return name + "\x00" + dynasty, nil
}

// canonicalAuthorName 把作者名归一到一个在简繁两表中都不会撞名的写法：先转简体，
// 再经繁体转回简体。
//
// 只转简体不够：源数据里同一人常有异体写法，如「朱庆余」与「朱庆馀」、「曹稆孙」与「曹穞孙」。
// 它们的简体不同，在简体表里是两位作者；转成繁体却都是「朱慶餘」「曹穭孫」，
// 繁体表按名字 + 朝代只能存一条。两表的作者因此对不上，同一首诗在简体表里出现两次，
// 在繁体表里只剩一首，诗词 ID 也随之错开。往返一次后，繁体相同的写法归为同一个键。
func canonicalAuthorName(author string) (string, error) {
	simplified, err := classifier.ToSimplified(author)
	if err != nil {
		return "", fmt.Errorf("failed to convert author %q: %w", author, err)
	}
	traditional, err := classifier.ToTraditional(simplified)
	if err != nil {
		return "", fmt.Errorf("failed to convert author %q: %w", author, err)
	}
	name, err := classifier.ToSimplified(traditional)
	if err != nil {
		return "", fmt.Errorf("failed to convert author %q: %w", author, err)
	}
	return name, nil
}

// Process 以多 worker 并发处理全部诗词，并批量写入数据库。
func (p *Processor) Process(poems []loader.PoemWithMeta) error {
	total := len(poems)
	logger.Info("Processing poems",
		zap.Int("total", total),
		zap.Int("workers", p.workers),
		zap.Int("batch_size", p.batchSize),
	)

	// 启动 worker 前先预热缓存，避免冷缓存下集中冲击数据库
	sourceTrad, err := detectSourceScripts(poems)
	if err != nil {
		return err
	}

	if err := p.prewarmCache(poems, sourceTrad); err != nil {
		return fmt.Errorf("failed to pre-warm cache: %w", err)
	}

	// 进度条容器
	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(100*time.Millisecond),
	)

	bar := progress.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name("Processing: ", decor.WC{W: 12, C: decor.DindentRight}),
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 5}),
			decor.Name(" | "),
			decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 6}),
			decor.Name(" | "),
			decor.AverageSpeed(0, "%.0f poems/s", decor.WC{W: 12}),
		),
	)

	// 任务分发用的 channel，缓冲区大小随机器配置自适应
	workBuffer, resultBuffer, errorBuffer, _, _, _ := getOptimalConfig()

	workCh := make(chan PoemWork, workBuffer)
	resultCh := make(chan *database.Poem, resultBuffer)
	errorCh := make(chan error, errorBuffer)
	var wg sync.WaitGroup

	// 进度计数
	var processed atomic.Int64
	var errorCount atomic.Int64

	// 启动 worker 处理诗词（CPU 密集型）
	for i := range p.workers {
		wg.Go(func() {
			for work := range workCh {
				poem, err := p.processPoem(work)
				if err != nil {
					errorCount.Add(1)
					// 非阻塞记录错误
					select {
					case errorCh <- fmt.Errorf("worker %d: %s - %w", i, work.Title, err):
					default:
						// 通道已满则丢弃，避免阻塞
					}
					processed.Add(1)
					bar.Increment()
					continue
				}

				// 跳过 nil（如归一化后正文为空的条目）
				if poem == nil {
					processed.Add(1)
					bar.Increment()
					continue
				}

				resultCh <- poem
				processed.Add(1)
				bar.Increment()
			}
		})
	}

	// 启动批量写入 goroutine
	insertDone := make(chan error, 1)
	go func() {
		insertDone <- p.batchInserter(resultCh)
	}()

	// 分发任务
	go func() {
		for i, poem := range poems {
			workCh <- PoemWork{
				PoemWithMeta:      poem,
				ID:                int64(i + 1), // 从 1 开始的顺序 ID
				SourceTraditional: sourceTrad[i],
			}
		}
		close(workCh)
	}()

	wg.Wait()

	// 写入阶段开始前，先让处理进度条收尾
	bar.SetTotal(int64(total), true) // 标记为已完成
	progress.Wait()                  // 等待进度条渲染结束

	close(resultCh) // 通知批量写入协程收尾

	if err := <-insertDone; err != nil {
		return fmt.Errorf("batch insertion failed: %w", err)
	}

	close(errorCh)

	// 收集错误（此时通道已关闭，不会阻塞）
	var errors []error
	for err := range errorCh {
		errors = append(errors, err)
		if len(errors) >= MaxErrorsToCollect {
			break
		}
	}

	// 输出汇总结果
	successCount := processed.Load()
	failCount := errorCount.Load()

	if failCount > 0 {
		logger.Warn("Processing completed with errors",
			zap.Int64("success", successCount-failCount),
			zap.Int64("failed", failCount),
			zap.Int("total", total),
		)
		if len(errors) > 0 {
			for i := range min(len(errors), SampleErrorCount) {
				logger.Debug("Sample error", zap.Int("index", i+1), zap.Error(errors[i]))
			}
		}
		return fmt.Errorf("processing completed with %d errors", failCount)
	}

	logger.Info("Processing completed successfully", zap.Int("total", total))
	return nil
}

// batchInserter 汇总处理完的诗词，用大事务批量写库。
// 把大量 INSERT 合并到少数几个事务里，可显著降低 fsync 开销。
func (p *Processor) batchInserter(resultCh <-chan *database.Poem) error {
	// 先收齐所有已处理的诗词，顺带过滤 nil 作为兜底
	allPoems := make([]*database.Poem, 0, cap(resultCh))

	for poem := range resultCh {
		if poem != nil {
			allPoems = append(allPoems, poem)
		}
	}

	if len(allPoems) == 0 {
		return nil
	}

	// 整体去重、词题去歧（需要看到全部诗词才能判断）
	allPoems, removed := finalizePoems(allPoems)
	logger.Info("Batch inserter starting", zap.Int("poems", len(allPoems)), zap.Int("duplicates_removed", removed))

	// 写入阶段单独用一个进度条容器
	progress := mpb.New(
		mpb.WithWidth(60),
		mpb.WithRefreshRate(100*time.Millisecond),
	)

	// 每个事务写入 2 万首以减少 fsync 次数，
	// 事务内部再按当前配置的 batchSize 分批 INSERT
	transactionSize := 20000

	err := p.repo.BatchInsertPoemsWithTransaction(allPoems, transactionSize, p.batchSize, progress)

	progress.Wait() // 等待进度条渲染结束

	if err != nil {
		return fmt.Errorf("failed to insert poems with transactions: %w", err)
	}

	logger.Info("Batch insertion complete", zap.Int("inserted", len(allPoems)))
	return nil
}

// resolveTitleByCategory 依据诗词类别决定最终标题，不同类别取自不同的源字段：
//   - 词：取词牌名（rhythmic），若另有标题则拼成「词牌名·副标题」
//   - 论语 / 四书五经：取章节名（chapter）
//   - 其余（诗、曲、诗经、楚辞、蒙学等）：直接取标题
func resolveTitleByCategory(poem loader.PoemData, category string) string {
	switch category {
	case "词", "宋词": // 宋词、五代词，以词牌名为主标题
		if poem.Rhythmic != "" {
			if poem.Title != "" && poem.Title != poem.Rhythmic {
				return poem.Rhythmic + "·" + poem.Title
			}
			return poem.Rhythmic
		}
		return poem.Title // 无词牌名时回退到标题

	case "论语", "四书五经":
		if poem.Chapter != "" {
			return poem.Chapter
		}
		return poem.Title // 无章节名时回退到标题

	default: // 唐诗、元曲、诗经、楚辞、蒙学等
		return poem.Title
	}
}

// processPoem 把单条原始数据加工成可入库的 Poem，
// 返回 (nil, nil) 表示该条目应被静默跳过。

func (p *Processor) processPoem(work PoemWork) (*database.Poem, error) {
	poem := work.PoemData

	// 归一化各文本字段（去除首尾空白）。
	// NormalizeAndSplitParagraphs 还会拆分被合并成一句的正文
	// （如 "A。B。" → ["A。","B。"]）。
	author := classifier.NormalizeText(poem.Author)
	paragraphs := classifier.NormalizeAndSplitParagraphs(poem.Paragraphs)
	rhythmic := classifier.NormalizeText(poem.Rhythmic)

	// 归一化后正文为空则跳过
	if len(paragraphs) == 0 {
		return nil, nil
	}

	// 跳过占位正文（无正文。/ 無正文。/ 空。）
	if classifier.IsPlaceholderContent(paragraphs) {
		return nil, nil
	}

	if author == "" {
		author = "佚名"
	}
	// 只要有正文即可入库，允许没有正式标题

	// 作者身份须在转换前、按源数据的写法求得，见 planAuthors
	identity, err := authorIdentity(author, work.Dynasty)
	if err != nil {
		return nil, err
	}
	// 去重同样按归一后的名字判断，否则异体写法下的同一首诗在简体表里会保留两份
	authorName, err := canonicalAuthorName(author)
	if err != nil {
		return nil, err
	}

	// 正文哈希：规整（去标点、空白）后计算，供唯一索引与 finalizePoems 去重使用。
	// 一律按简体正文计算，与语言变体无关：同一首诗常同时收在繁体书写的全唐诗与
	// 简体书写的选本（唐诗三百首等）里，繁体库保留前者原文、后者由简转繁得来，
	// 两者用字未必完全相同（游／遊）。按各自变体的正文去重，两张表会保留不同的诗。
	hashSource := paragraphs
	if work.SourceTraditional {
		if hashSource, err = classifier.ToSimplifiedArray(paragraphs); err != nil {
			return nil, fmt.Errorf("failed to convert paragraphs for hashing: %w", err)
		}
	}
	hash := contentHash(hashSource)

	// 统一简繁：繁体库转繁体，简体库转简体
	author, err = p.toVariant(author, work.SourceTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert author: %w", err)
	}

	paragraphs, err = p.toVariantArray(paragraphs, work.SourceTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert paragraphs: %w", err)
	}

	if rhythmic != "" {
		rhythmic, err = p.toVariant(rhythmic, work.SourceTraditional)
		if err != nil {
			return nil, fmt.Errorf("failed to convert rhythmic: %w", err)
		}
	}

	// 朝代名同样要转成与目标库一致的简繁形式
	dynastyName, err := p.convertText(work.Dynasty, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert dynasty name: %w", err)
	}
	dynastyID, err := p.repo.GetOrCreateDynasty(dynastyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create dynasty: %w", err)
	}

	// 预热时已规划的作者直接取其 ID；只有预热未覆盖到的（理论上不会出现）才按名字建
	authorID, ok := p.authorIDs[identity]
	if !ok {
		authorID, err = p.repo.GetOrCreateAuthor(author, dynastyID)
		if err != nil {
			return nil, fmt.Errorf("failed to get/create author: %w", err)
		}
	}

	// 结合数据集来源与标题判定诗词体裁
	typeInfo := classifier.ClassifyPoetryTypeWithDataset(paragraphs, rhythmic, work.DatasetKey, poem.Title)

	typeName, err := p.convertText(typeInfo.TypeName, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert type name: %w", err)
	}

	typeID, err := p.repo.GetPoetryTypeID(typeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get poetry type: %w", err)
	}

	// 按类别在 title / rhythmic / chapter 之间挑选最终标题
	finalTitle := resolveTitleByCategory(poem, typeInfo.Category)

	finalTitle, err = p.toVariant(finalTitle, work.SourceTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert final title: %w", err)
	}

	// 使用分发阶段分配的顺序 ID
	poemID := work.ID

	// 正文以 JSON 数组形式存储
	contentJSON, err := json.Marshal(paragraphs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal paragraphs: %w", err)
	}

	// 无副标题的词记下首句，同一作者同一词牌有多首时用于区分标题
	firstLine := ""
	if (typeInfo.Category == "词" || typeInfo.Category == "宋词") && rhythmic != "" &&
		(poem.Title == "" || poem.Title == poem.Rhythmic) {
		firstLine = firstClause(paragraphs)
	}

	dbPoem := &database.Poem{
		ID:          poemID,
		Title:       finalTitle, // 按类别选出的标题，可能来自 title/rhythmic/chapter
		AuthorID:    &authorID,
		DynastyID:   &dynastyID,
		TypeID:      &typeID,
		Content:     datatypes.JSON(contentJSON),
		ContentHash: hash,
		FirstLine:   firstLine,
		AuthorName:  authorName,
	}

	return dbPoem, nil
}

// toVariant 把源文本转成本语言变体，源文本本就是目标字形时原样返回。
//
// 不能无条件转换：简转繁对已是繁体的文本并非恒等，简繁同形的字会被改掉
// （十里→十裏、咸→鹹、陸游→陸遊）。此前繁体库把繁体书写的全唐诗整体再做一遍
// 简转繁，抽样中约 35% 的诗正文被改坏。反方向（繁转简作用于简体文本）同理。
// 朝代名、体裁名等代码里写死的简体常量不是源文本，仍用 convertText。
func (p *Processor) toVariant(text string, sourceTraditional bool) (string, error) {
	if sourceTraditional == p.convertToTraditional {
		return text, nil
	}
	return p.convertText(text, p.convertToTraditional)
}

// toVariantArray 是 toVariant 的批量版本。
func (p *Processor) toVariantArray(texts []string, sourceTraditional bool) ([]string, error) {
	if sourceTraditional == p.convertToTraditional {
		return texts, nil
	}
	return p.convertTextArray(texts, p.convertToTraditional)
}

// detectSourceScripts 逐首判断源文本是否以繁体书写。
// 按首而不按数据集判断：全唐诗整体为繁体，但其中也混有少量简体记录。
func detectSourceScripts(poems []loader.PoemWithMeta) ([]bool, error) {
	out := make([]bool, len(poems))
	var sb strings.Builder
	for i, poem := range poems {
		sb.Reset()
		sb.WriteString(poem.Title)
		sb.WriteString(poem.Rhythmic)
		sb.WriteString(poem.Author)
		for _, para := range poem.Paragraphs {
			sb.WriteString(para)
		}
		trad, err := classifier.IsTraditional(sb.String())
		if err != nil {
			return nil, fmt.Errorf("failed to detect script of %q: %w", poem.Title, err)
		}
		out[i] = trad
	}
	return out, nil
}

// convertText 按 toTraditional 标志把文本转为繁体或简体。
func (p *Processor) convertText(text string, toTraditional bool) (string, error) {
	if toTraditional {
		return classifier.ToTraditional(text)
	}
	return classifier.ToSimplified(text)
}

// convertTextArray 按 toTraditional 标志批量转换文本的简繁形式。
func (p *Processor) convertTextArray(texts []string, toTraditional bool) ([]string, error) {
	if toTraditional {
		return classifier.ToTraditionalArray(texts)
	}
	return classifier.ToSimplifiedArray(texts)
}
