package database

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// countCache 缓存分页查询附带的 COUNT 结果。
//
// 每次分页请求都要跑一遍 COUNT 才能算出 total/total_pages，而这个代价与页码无关，
// 第 1 页也照收。在 30 万首的语料上，搜索的 COUNT 约 55ms——它要把同一个 FTS join
// 再跑一遍，是这里最值得缓存的一类。
//
// 无过滤的诗词总数不走这里，它读的是触发器维护的计数器，见 DB.migratePoemCounter。
//
// 语料在服务运行期间是只读的（数据由 processor 离线导入），因此这里用一个
// 朴素的 TTL 缓存即可；导入路径上的写操作会显式调用 invalidate。
type countCache struct {
	mu      sync.Mutex
	entries map[string]countEntry
}

type countEntry struct {
	value     int64
	expiresAt time.Time
}

const (
	// countCacheTTL 是缓存项的存活时间。取值不必长：COUNT 本身不贵在单次，
	// 贵在每个请求都跑一次，几分钟的 TTL 已经能吸收绝大部分重复。
	countCacheTTL = 5 * time.Minute

	// countCacheMaxEntries 限制缓存规模。搜索的缓存键含用户输入的关键词，
	// 键空间由外部控制，没有上限的话可以被构造成内存放大器。
	// 超过上限时整体清空——比实现 LRU 简单得多，而条目本身重建代价可控。
	countCacheMaxEntries = 1024
)

// getOrLoad 返回 key 对应的计数，未命中或已过期时调用 load 重新取值。
//
// load 在锁外执行：COUNT 可能耗时几十毫秒，持锁执行会让并发请求排成一队，
// 反而比不加缓存更糟。代价是同一个 key 在冷启动时可能被并发加载多次，
// 这只是重复做了本来就要做的工作，结果一致，可以接受。
func (c *countCache) getOrLoad(key string, load func() (int64, error)) (int64, error) {
	if value, ok := c.get(key); ok {
		return value, nil
	}

	value, err := load()
	if err != nil {
		return 0, err
	}

	c.set(key, value)
	return value, nil
}

func (c *countCache) get(key string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return 0, false
	}
	return entry.value, true
}

func (c *countCache) set(key string, value int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]countEntry)
	}
	if len(c.entries) >= countCacheMaxEntries {
		c.entries = make(map[string]countEntry)
	}

	c.entries[key] = countEntry{
		value:     value,
		expiresAt: time.Now().Add(countCacheTTL),
	}
}

// invalidate 清空全部缓存，供写入路径在数据变更后调用。
func (c *countCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// InvalidateCountCache 在诗词数据发生变更后清空计数缓存。
func (db *DB) InvalidateCountCache() {
	db.counts.invalidate()
}

// countKeyBuilder 拼装计数缓存的键。
//
// 键必须能唯一确定一条 COUNT 查询，因此除了过滤条件本身，还要带上语言变体
// （简繁是两套独立的表）。各段之间用 \x00 分隔，避免 "作者=李白|体裁=" 这类
// 取值拼接后与另一组取值撞键。
type countKeyBuilder struct {
	sb strings.Builder
}

func newCountKey(scope string, lang Lang) *countKeyBuilder {
	b := &countKeyBuilder{}
	b.sb.WriteString(scope)
	b.add(string(lang))
	return b
}

func (b *countKeyBuilder) add(s string) *countKeyBuilder {
	b.sb.WriteByte(0)
	b.sb.WriteString(s)
	return b
}

func (b *countKeyBuilder) addOptionalID(id *int64) *countKeyBuilder {
	if id == nil {
		return b.add("")
	}
	return b.add(strconv.FormatInt(*id, 10))
}

func (b *countKeyBuilder) addID(id int64) *countKeyBuilder {
	return b.add(strconv.FormatInt(id, 10))
}

func (b *countKeyBuilder) addIDs(ids []int64) *countKeyBuilder {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return b.add(strings.Join(parts, ","))
}

func (b *countKeyBuilder) String() string {
	return b.sb.String()
}
