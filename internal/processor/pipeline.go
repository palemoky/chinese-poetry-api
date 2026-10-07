package processor

import (
	"encoding/json"
	"fmt"
	"runtime"
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
func (p *Processor) prewarmCache(poems []loader.PoemWithMeta) error {
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

	// 收集去重后的作者（约一万条，仍可接受）。
	// 作者依赖朝代 ID，所以必须在朝代缓存预热之后执行。
	authorSet := make(map[string]string) // 作者名 -> 朝代名
	for _, poem := range poems {
		author := classifier.NormalizeText(poem.Author)
		if author == "" {
			author = "佚名"
		}
		converted, err := p.convertText(author, p.convertToTraditional)
		if err != nil {
			continue
		}
		if _, exists := authorSet[converted]; !exists {
			dynasty := poem.Dynasty
			if dynasty != "" {
				dynasty, _ = p.convertText(dynasty, p.convertToTraditional)
			}
			authorSet[converted] = dynasty
		}
	}

	// 预热作者缓存
	for author, dynasty := range authorSet {
		var dynastyID int64 = 0
		if dynasty != "" {
			var err error
			dynastyID, err = p.repo.GetOrCreateDynasty(dynasty)
			if err != nil {
				continue // 留到正式处理阶段再报
			}
		}
		if _, err := p.repo.GetOrCreateAuthor(author, dynastyID); err != nil {
			continue // 预热失败不影响流程，正式处理时会重试
		}
	}

	logger.Info("Cache pre-warmed",
		zap.Int("dynasties", len(dynastySet)),
		zap.Int("authors", len(authorSet)),
	)

	return nil
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
	if err := p.prewarmCache(poems); err != nil {
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
				PoemWithMeta: poem,
				ID:           int64(i + 1), // 从 1 开始的顺序 ID
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

	// 统一简繁：繁体库转繁体，简体库转简体
	author, err := p.convertText(author, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert author: %w", err)
	}

	paragraphs, err = p.convertTextArray(paragraphs, p.convertToTraditional)
	if err != nil {
		return nil, fmt.Errorf("failed to convert paragraphs: %w", err)
	}

	if rhythmic != "" {
		rhythmic, err = p.convertText(rhythmic, p.convertToTraditional)
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

	authorID, err := p.repo.GetOrCreateAuthor(author, dynastyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create author: %w", err)
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

	finalTitle, err = p.convertText(finalTitle, p.convertToTraditional)
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

	// 正文哈希：规整（去标点、空白）后计算，供唯一索引与 finalizePoems 去重使用
	hash := contentHash(paragraphs)

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
	}

	return dbPoem, nil
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
