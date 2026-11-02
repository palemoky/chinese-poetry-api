package processor

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/palemoky/chinese-poetry-api/internal/classifier"
	"github.com/palemoky/chinese-poetry-api/internal/database"
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

// anthologyBits 把选本文件对应到数据库里的选本标记。
var anthologyBits = map[string]int{
	"tangshi300": database.AnthologyTangShi300,
	"qianjiashi": database.AnthologyQianJiaShi,
	"songci300":  database.AnthologySongCi300,
}

// 选本里的诗与全集的版本常有异文，正文哈希对不上，因此按句子重合来找。
// 规则与 eval/semantic/signals.py 相同，那里在全量数据上核对过：
// 《唐诗三百首》320 首对上 318 首，《宋词三百首》280 首全部对上。
const (
	minLineRunes = 4 // 更短的句子太容易撞上别的诗
	// maxLineOwners 以上的诗共有的句子是套语（如“一年三百六十日”），不能说明是同一首诗
	maxLineOwners   = 30
	minSharedLines  = 2
	minShareOfLines = 0.4 // 至少要有这么多比例的句子相同
	// 同一首诗常有多个版本（标题或作者不同），票数接近最高者的都算
	keepShareOfBest = 0.7
)

type anthologyEntry struct {
	bit   int
	lines []string
}

// SetAnthologies 设置用来标记名篇的选本诗词。正文先转成简体，与 Poem.SimplifiedContent 比较。
func (p *Processor) SetAnthologies(poems []loader.AnthologyPoem) error {
	entries := make([]anthologyEntry, 0, len(poems))
	for _, poem := range poems {
		bit, ok := anthologyBits[poem.Source]
		if !ok {
			return fmt.Errorf("unknown anthology %q", poem.Source)
		}
		simplified, err := classifier.ToSimplifiedArray(poem.Paragraphs)
		if err != nil {
			return fmt.Errorf("failed to convert anthology poem: %w", err)
		}
		if lines := matchLines(simplified); len(lines) > 0 {
			entries = append(entries, anthologyEntry{bit: bit, lines: lines})
		}
	}
	p.anthologies = entries
	return nil
}

// matchLines 把正文切成句子，只保留汉字，去掉过短的句子与重复的句子。
func matchLines(paragraphs []string) []string {
	seen := make(map[string]struct{})
	var lines []string
	for _, paragraph := range paragraphs {
		for _, part := range strings.FieldsFunc(paragraph, func(r rune) bool {
			return unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r)
		}) {
			line := strings.Map(func(r rune) rune {
				if unicode.Is(unicode.Han, r) {
					return r
				}
				return -1
			}, part)
			if len([]rune(line)) < minLineRunes {
				continue
			}
			if _, dup := seen[line]; !dup {
				seen[line] = struct{}{}
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// markAnthologies 在 poems 中找出选本里的诗，把对应的位写进 Anthology，
// 返回每个选本对上的诗数（选本中的首数，而不是标记的诗词数）。
//
// 只为选本里出现过的句子建索引，不必为全集的数百万个句子建表。
func markAnthologies(poems []*database.Poem, entries []anthologyEntry) map[int]int {
	matched := make(map[int]int)
	if len(entries) == 0 {
		return matched
	}

	owners := make(map[string][]int)
	for _, e := range entries {
		for _, line := range e.lines {
			owners[line] = nil
		}
	}
	for i, poem := range poems {
		for _, line := range matchLines(poem.SimplifiedContent) {
			// 超过上限的句子不会再用到，记到上限加一即可
			if list, ok := owners[line]; ok && len(list) <= maxLineOwners {
				owners[line] = append(list, i)
			}
		}
	}

	for _, e := range entries {
		votes := make(map[int]int)
		for _, line := range e.lines {
			if list := owners[line]; len(list) <= maxLineOwners {
				for _, i := range list {
					votes[i]++
				}
			}
		}

		best := 0
		for _, v := range votes {
			best = max(best, v)
		}
		if float64(best) < math.Max(minSharedLines, minShareOfLines*float64(len(e.lines))) {
			continue
		}
		keep := math.Max(minSharedLines, keepShareOfBest*float64(best))
		for i, v := range votes {
			if float64(v) >= keep {
				poems[i].Anthology |= e.bit
			}
		}
		matched[e.bit]++
	}
	return matched
}
