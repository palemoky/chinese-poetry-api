package processor

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// contentHash 对正文做规整后取 SHA256：只保留文字与数字，去掉标点和空白。
// 这样仅标点、断句或拆分方式不同的同一首作品（如 "A。B。" 与 ["A，","B。"]）哈希相同。
func contentHash(paragraphs []string) string {
	var b strings.Builder
	for _, p := range paragraphs {
		for _, r := range p {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				b.WriteRune(r)
			}
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// firstClause 返回正文的首句（首段中第一个标点之前的部分）。
func firstClause(paragraphs []string) string {
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i := strings.IndexFunc(p, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) }); i > 0 {
			return p[:i]
		}
		return p
	}
	return ""
}

type dedupKey struct {
	author string
	hash   string
}

type titleKey struct {
	authorID int64
	title    string
}

// dedupAuthor 返回去重时用于判断「同一作者」的键：有作者名时用名字，否则退回作者 ID。
func dedupAuthor(p *database.Poem) string {
	if p.AuthorName != "" {
		return p.AuthorName
	}
	return "#" + strconv.FormatInt(authorOf(p), 10)
}

func authorOf(p *database.Poem) int64 {
	if p.AuthorID == nil {
		return 0
	}
	return *p.AuthorID
}

// finalizePoems 在入库前对全部诗词做整体处理（就地修改并返回保留的条目）：
//
//  1. 去重：同一作者、正文规整后相同的只保留 ID 最小的一条（即数据集顺序中最先出现的）。
//     作者不同的同文作品（如《全唐诗》中的重出诗）属于归属异说，予以保留。
//     「同一作者」按作者名而非作者 ID 判断，见 database.Poem.AuthorName。
//  2. 词题去歧：同一作者同一词牌、且都没有副标题的多首词，标题改为「词牌·首句」，
//     如 4 首「水调歌头」→「水调歌头·明月几时有」等；只有一首时保持原样。
//
// 返回值按 ID 升序。
func finalizePoems(poems []*database.Poem) (kept []*database.Poem, removed int) {
	sort.Slice(poems, func(i, j int) bool { return poems[i].ID < poems[j].ID })

	seen := make(map[dedupKey]struct{}, len(poems))
	kept = poems[:0]
	for _, p := range poems {
		k := dedupKey{dedupAuthor(p), p.ContentHash}
		if _, dup := seen[k]; dup {
			removed++
			continue
		}
		seen[k] = struct{}{}
		kept = append(kept, p)
	}

	groups := make(map[titleKey][]*database.Poem)
	for _, p := range kept {
		if p.FirstLine != "" {
			k := titleKey{authorOf(p), p.Title}
			groups[k] = append(groups[k], p)
		}
	}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		for _, p := range g {
			p.Title = p.Title + "·" + p.FirstLine
		}
	}
	return kept, removed
}
