package database

import (
	"fmt"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/palemoky/chinese-poetry-api/internal/logger"
)

// 本文件包含数据导入阶段使用的写操作。

// GetOrCreateDynasty 按名称查询或创建朝代，可安全并发调用。
// 借助 ON CONFLICT 来化解并发插入冲突。
func (r *Repository) GetOrCreateDynasty(name string) (int64, error) {
	dynasty := Dynasty{Name: name}

	// 以 ON CONFLICT DO NOTHING 的方式尝试插入
	err := r.db.Table(r.dynastiesTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true, // 已存在则忽略
	}).Create(&dynasty).Error
	if err != nil {
		return 0, err
	}

	// ID 为 0 说明插入被跳过（记录已存在），需要回查已有记录
	if dynasty.ID == 0 {
		err = r.db.Table(r.dynastiesTable()).Where("name = ?", name).First(&dynasty).Error
		if err != nil {
			return 0, err
		}
	}

	return dynasty.ID, nil
}

// GetOrCreateAuthor 查询或创建作者，可安全并发调用。
// 以 name 为唯一键，并借助 ON CONFLICT 化解并发插入冲突。
// 注意：dynasty_id 只在首次创建时写入，之后不再更新，
// 因为同一位作者可能出现在多个朝代的数据集中。
func (r *Repository) GetOrCreateAuthor(name string, dynastyID int64) (int64, error) {
	author := Author{
		Name:      name,
		DynastyID: &dynastyID,
	}

	// 以 ON CONFLICT DO NOTHING 的方式尝试插入
	err := r.db.Table(r.authorsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true, // 已存在则忽略
	}).Create(&author).Error
	if err != nil {
		return 0, err
	}

	// ID 为 0 说明插入被跳过（记录已存在），需要回查已有记录
	if author.ID == 0 {
		err = r.db.Table(r.authorsTable()).Where("name = ?", name).First(&author).Error
		if err != nil {
			return 0, err
		}
	}

	return author.ID, nil
}

// GetPoetryTypeID 按名称查询体裁 ID。
func (r *Repository) GetPoetryTypeID(name string) (int64, error) {
	var poetryType PoetryType
	err := r.db.Table(r.poetryTypesTable()).Where("name = ?", name).First(&poetryType).Error
	if err != nil {
		return 0, err
	}
	return poetryType.ID, nil
}

// GetPoetryTypeIDs 用一次查询批量获取多个体裁的 ID，
// 返回顺序与传入的名称一致；任一名称查不到时返回错误。
func (r *Repository) GetPoetryTypeIDs(names []string) ([]int64, error) {
	if len(names) == 0 {
		return []int64{}, nil
	}

	var poetryTypes []PoetryType
	err := r.db.Table(r.poetryTypesTable()).
		Where("name IN ?", names).
		Find(&poetryTypes).Error
	if err != nil {
		return nil, err
	}

	// 注意：这里刻意不去比较返回行数与 len(names)。
	// 重复的名称（如 ?type=五言绝句&type=五言绝句）在 IN 子句中会合并成一行，
	// 若按行数比较会误拒完全合法的请求。下面逐名查表时已能识别真正不存在的名称。

	// 建映射表以便 O(1) 查找
	typeMap := make(map[string]int64, len(poetryTypes))
	for _, pt := range poetryTypes {
		typeMap[pt.Name] = pt.ID
	}

	// 按输入名称的顺序返回 ID
	ids := make([]int64, len(names))
	for i, name := range names {
		id, ok := typeMap[name]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		ids[i] = id
	}

	return ids, nil
}

// InsertPoem 插入单首诗词。
func (r *Repository) InsertPoem(poem *Poem) error {
	if err := r.db.Table(r.poemsTable()).Create(poem).Error; err != nil {
		return err
	}
	// 诗词数量变了，缓存的 COUNT 结果随之失效
	r.db.counts.invalidate()
	return nil
}

// BatchInsertPoems 分批插入诗词以提升性能，重复记录会被跳过。
func (r *Repository) BatchInsertPoems(poems []*Poem, batchSize int) error {
	if len(poems) == 0 {
		return nil
	}

	if batchSize <= 0 {
		batchSize = 100 // 默认批量大小
	}

	// 用 CreateInBatches 配合 OnConflict 处理重复，
	// 依据 (title, content_hash) 复合唯一索引跳过重复记录
	err := r.db.Table(r.poemsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "title"}, {Name: "content_hash"}},
		DoNothing: true, // 跳过重复记录
	}).CreateInBatches(poems, batchSize).Error
	if err != nil {
		return err
	}

	r.db.counts.invalidate()
	return nil
}

// BatchInsertPoemsWithTransaction 用大事务批量写入诗词以获得最佳性能，
// 把多个批次合并进同一个事务可显著降低 fsync 开销。
// transactionSize：每个事务写入的诗词数（如 10000）
// batchSize：单条 INSERT 语句写入的诗词数（如 1000）
// progress：用于展示写入进度的进度条容器
func (r *Repository) BatchInsertPoemsWithTransaction(poems []*Poem, transactionSize, batchSize int, progress *mpb.Progress) error {
	if len(poems) == 0 {
		return nil
	}

	if transactionSize <= 0 {
		transactionSize = 20000 // 默认每个事务 2 万首
	}
	if batchSize <= 0 {
		batchSize = 1000 // 默认每条 INSERT 一千首
	}

	totalTransactions := (len(poems) + transactionSize - 1) / transactionSize

	// 进度条按诗词数而非事务数计量，刷新更平滑
	var poemBar *mpb.Bar
	if progress != nil {
		poemBar = progress.AddBar(int64(len(poems)),
			mpb.PrependDecorators(
				decor.Name("Inserting Poems: ", decor.WC{W: 17, C: decor.DindentRight}),
				decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
			),
			mpb.AppendDecorators(
				decor.Percentage(decor.WC{W: 5}),
				decor.Name(" | "),
				decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 6}),
			),
		)
	}

	logger.Info("Starting batch insertion",
		zap.Int("poems", len(poems)),
		zap.Int("transactions", totalTransactions),
		zap.Int("batch_size", batchSize),
	)

	// 按事务粒度切分并逐块写入
	for i := 0; i < len(poems); i += transactionSize {
		end := min(i+transactionSize, len(poems))
		transactionChunk := poems[i:end]

		// 单个大事务内手动分批，以便刷新进度条
		err := r.db.Transaction(func(tx *gorm.DB) error {
			for j := 0; j < len(transactionChunk); j += batchSize {
				batchEnd := min(j+batchSize, len(transactionChunk))
				batch := transactionChunk[j:batchEnd]

				// 写入当前批次，重复记录自动跳过
				err := tx.Table(r.poemsTable()).Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "title"}, {Name: "content_hash"}},
					DoNothing: true,
				}).Create(&batch).Error
				if err != nil {
					return err
				}

				// 每批写完刷新一次进度
				if poemBar != nil {
					poemBar.IncrBy(len(batch))
				}
			}
			return nil
		})
		if err != nil {
			txNum := i/transactionSize + 1
			return fmt.Errorf("failed to insert transaction %d/%d (poems %d-%d): %w",
				txNum, totalTransactions, i, end, err)
		}
	}

	return nil
}

// UpsertPoem 插入诗词，若已存在则更新（用于处理重复数据）。
func (r *Repository) UpsertPoem(poem *Poem) error {
	err := r.db.Table(r.poemsTable()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "content", "author_id", "dynasty_id", "type_id"}),
	}).Create(poem).Error
	if err != nil {
		return err
	}

	r.db.counts.invalidate()
	return nil
}
