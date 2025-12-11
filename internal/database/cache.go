package database

import (
	"sync"
)

// CachedRepository wraps Repository with caching for frequently accessed data
type CachedRepository struct {
	*Repository

	// Caches for frequently accessed data
	dynastyCache   map[string]int64
	dynastyCacheMu sync.RWMutex

	typeCache   map[string]int64
	typeCacheMu sync.RWMutex

	authorCache   map[string]int64
	authorCacheMu sync.RWMutex
}

// NewCachedRepository creates a new cached repository
func NewCachedRepository(repo *Repository) *CachedRepository {
	return &CachedRepository{
		Repository:   repo,
		dynastyCache: make(map[string]int64),
		typeCache:    make(map[string]int64),
		authorCache:  make(map[string]int64),
	}
}

// GetOrCreateDynasty gets or creates a dynasty with caching
func (r *CachedRepository) GetOrCreateDynasty(name string) (int64, error) {
	// Try to get from cache first
	r.dynastyCacheMu.RLock()
	if id, ok := r.dynastyCache[name]; ok {
		r.dynastyCacheMu.RUnlock()
		return id, nil
	}
	r.dynastyCacheMu.RUnlock()

	// Not in cache, get from database
	id, err := r.Repository.GetOrCreateDynasty(name)
	if err != nil {
		return 0, err
	}

	// Store in cache
	r.dynastyCacheMu.Lock()
	r.dynastyCache[name] = id
	r.dynastyCacheMu.Unlock()

	return id, nil
}

// GetPoetryTypeID gets the ID of a poetry type with caching
func (r *CachedRepository) GetPoetryTypeID(name string) (int64, error) {
	// Try to get from cache first
	r.typeCacheMu.RLock()
	if id, ok := r.typeCache[name]; ok {
		r.typeCacheMu.RUnlock()
		return id, nil
	}
	r.typeCacheMu.RUnlock()

	// Not in cache, get from database
	id, err := r.Repository.GetPoetryTypeID(name)
	if err != nil {
		return 0, err
	}

	// Store in cache
	r.typeCacheMu.Lock()
	r.typeCache[name] = id
	r.typeCacheMu.Unlock()

	return id, nil
}

// GetOrCreateAuthor gets or creates an author with caching
func (r *CachedRepository) GetOrCreateAuthor(name string, dynastyID int64) (int64, error) {
	// Try to get from cache first (use name as key since it's unique)
	r.authorCacheMu.RLock()
	if id, ok := r.authorCache[name]; ok {
		r.authorCacheMu.RUnlock()
		return id, nil
	}
	r.authorCacheMu.RUnlock()

	// Not in cache, get from database
	id, err := r.Repository.GetOrCreateAuthor(name, dynastyID)
	if err != nil {
		return 0, err
	}

	// Store in cache
	r.authorCacheMu.Lock()
	r.authorCache[name] = id
	r.authorCacheMu.Unlock()

	return id, nil
}

// ClearCache clears all caches
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

// GetCacheStats returns statistics about cache usage
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
