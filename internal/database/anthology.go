package database

import (
	"fmt"
	"sync"
)

// 选本标记，按位存进 poems.anthology。值写进了发布的数据库，不能改动已有的位。
const (
	AnthologyTangShi300 = 1 << iota // 唐诗三百首
	AnthologyQianJiaShi             // 千家诗
	AnthologySongCi300              // 宋词三百首
)

// AnthologyNames 是各选本的名称，按位对应。
var AnthologyNames = map[int]string{
	AnthologyTangShi300: "唐诗三百首",
	AnthologyQianJiaShi: "千家诗",
	AnthologySongCi300:  "宋词三百首",
}

// anthologyCache 缓存每个语言变体下收在任一选本里的诗词 id（升序）。
// 只有约一千首，服务期间数据只读，读一次即可；导入写入时随其他缓存一起清空。
type anthologyCache struct {
	mu  sync.Mutex
	ids map[Lang][]int64
}

func (c *anthologyCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = nil
}

// AnthologyIDs 返回收在任一选本里的诗词 id（升序），供搜索给名篇加分。旧库补上的列全是 0，结果为空。
func (r *Repository) AnthologyIDs() ([]int64, error) {
	c := &r.db.anthologies
	c.mu.Lock()
	defer c.mu.Unlock()

	if ids, ok := c.ids[r.lang]; ok {
		return ids, nil
	}

	ids := []int64{}
	err := r.conn().Table(r.poemsTable()).Where("anthology != 0").Order("id").Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load anthology poems: %w", err)
	}
	if c.ids == nil {
		c.ids = make(map[Lang][]int64)
	}
	c.ids[r.lang] = ids
	return ids, nil
}
