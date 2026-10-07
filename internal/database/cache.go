package database

import (
	"sync"
)

// CachedRepository 在 Repository 之上叠加一层内存缓存，
// 用于导入阶段高频访问的朝代、体裁与作者 ID。
type CachedRepository struct {
	*Repository

	dynastyCache   map[string]int64
	dynastyCacheMu sync.RWMutex

	typeCache   map[string]int64
	typeCacheMu sync.RWMutex

	authorCache   map[string]int64
	authorCacheMu sync.RWMutex
}

// NewCachedRepository 创建带缓存的仓储。
func NewCachedRepository(repo *Repository) *CachedRepository {
	return &CachedRepository{
		Repository:   repo,
		dynastyCache: make(map[string]int64),
		typeCache:    make(map[string]int64),
		authorCache:  make(map[string]int64),
	}
}

// GetOrCreateDynasty 查询或创建朝代，结果带缓存。
func (r *CachedRepository) GetOrCreateDynasty(name string) (int64, error) {
	// 先查缓存
	r.dynastyCacheMu.RLock()
	if id, ok := r.dynastyCache[name]; ok {
		r.dynastyCacheMu.RUnlock()
		return id, nil
	}
	r.dynastyCacheMu.RUnlock()

	// 未命中则回落到数据库
	id, err := r.Repository.GetOrCreateDynasty(name)
	if err != nil {
		return 0, err
	}

	// 结果写入缓存
	r.dynastyCacheMu.Lock()
	r.dynastyCache[name] = id
	r.dynastyCacheMu.Unlock()

	return id, nil
}

// GetPoetryTypeID 查询体裁 ID，结果带缓存。
func (r *CachedRepository) GetPoetryTypeID(name string) (int64, error) {
	// 先查缓存
	r.typeCacheMu.RLock()
	if id, ok := r.typeCache[name]; ok {
		r.typeCacheMu.RUnlock()
		return id, nil
	}
	r.typeCacheMu.RUnlock()

	// 未命中则回落到数据库
	id, err := r.Repository.GetPoetryTypeID(name)
	if err != nil {
		return 0, err
	}

	// 结果写入缓存
	r.typeCacheMu.Lock()
	r.typeCache[name] = id
	r.typeCacheMu.Unlock()

	return id, nil
}

// GetPoetryTypeIDs 批量查询体裁 ID：先查缓存，仅对未命中的部分查库。
func (r *CachedRepository) GetPoetryTypeIDs(names []string) ([]int64, error) {
	if len(names) == 0 {
		return []int64{}, nil
	}

	ids := make([]int64, len(names))
	missingNames := []string{}
	missingIndices := []int{}

	// 逐个查缓存，记录未命中的名称及其下标
	r.typeCacheMu.RLock()
	for i, name := range names {
		if id, ok := r.typeCache[name]; ok {
			ids[i] = id
		} else {
			missingNames = append(missingNames, name)
			missingIndices = append(missingIndices, i)
		}
	}
	r.typeCacheMu.RUnlock()

	// 全部命中则直接返回
	if len(missingNames) == 0 {
		return ids, nil
	}

	// 未命中的部分查库补齐
	missingIDs, err := r.Repository.GetPoetryTypeIDs(missingNames)
	if err != nil {
		return nil, err
	}

	// 回填结果并更新缓存
	r.typeCacheMu.Lock()
	for i, name := range missingNames {
		id := missingIDs[i]
		r.typeCache[name] = id
		ids[missingIndices[i]] = id
	}
	r.typeCacheMu.Unlock()

	return ids, nil
}

// GetOrCreateAuthor 查询或创建作者，结果带缓存。
func (r *CachedRepository) GetOrCreateAuthor(name string, dynastyID int64) (int64, error) {
	// 先查缓存，作者名唯一，直接用作 key
	r.authorCacheMu.RLock()
	if id, ok := r.authorCache[name]; ok {
		r.authorCacheMu.RUnlock()
		return id, nil
	}
	r.authorCacheMu.RUnlock()

	// 未命中则回落到数据库
	id, err := r.Repository.GetOrCreateAuthor(name, dynastyID)
	if err != nil {
		return 0, err
	}

	// 结果写入缓存
	r.authorCacheMu.Lock()
	r.authorCache[name] = id
	r.authorCacheMu.Unlock()

	return id, nil
}

// CreateAuthorWithID 以指定 ID 创建作者，并把结果写入缓存，
// 之后按名字调用 GetOrCreateAuthor 会直接命中这个 ID。
func (r *CachedRepository) CreateAuthorWithID(id int64, name string, dynastyID int64) (int64, error) {
	got, err := r.Repository.CreateAuthorWithID(id, name, dynastyID)
	if err != nil {
		return 0, err
	}

	r.authorCacheMu.Lock()
	r.authorCache[name] = got
	r.authorCacheMu.Unlock()

	return got, nil
}

// ClearCache 清空全部缓存。
func (r *CachedRepository) ClearCache() {
	r.dynastyCacheMu.Lock()
	r.dynastyCache = make(map[string]int64)
	r.dynastyCacheMu.Unlock()

	r.typeCacheMu.Lock()
	r.typeCache = make(map[string]int64)
	r.typeCacheMu.Unlock()

	r.authorCacheMu.Lock()
	r.authorCache = make(map[string]int64)
	r.authorCacheMu.Unlock()
}

// GetCacheStats 返回各类缓存当前的条目数量。
func (r *CachedRepository) GetCacheStats() map[string]int {
	r.dynastyCacheMu.RLock()
	dynastyCount := len(r.dynastyCache)
	r.dynastyCacheMu.RUnlock()

	r.typeCacheMu.RLock()
	typeCount := len(r.typeCache)
	r.typeCacheMu.RUnlock()

	r.authorCacheMu.RLock()
	authorCount := len(r.authorCache)
	r.authorCacheMu.RUnlock()

	return map[string]int{
		"dynasties": dynastyCount,
		"types":     typeCount,
		"authors":   authorCount,
	}
}
