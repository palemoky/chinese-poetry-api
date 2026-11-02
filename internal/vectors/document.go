// Package vectors 定义语义检索用的向量文件（poetry-vectors.db）：每首诗一条 int8 向量。
//
// 向量文件与主库分开发布，是可选的：不需要语义检索的用户不必下载。
// 向量按“嵌入时使用的文本”的哈希对应到诗，而不只是按 ID：诗词 ID 在每次发布时都可能变化，
// 而文本没变的诗可以直接沿用上一版的向量，每次发布只需嵌入改动过的诗。
package vectors

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// TextVersion 是 Document 拼接规则的版本，写进向量文件。改动拼接规则必须加一，
// 旧向量随之作废，不能再被复用。
const TextVersion = 1

// maxDocumentRunes 与 eval/semantic/embed_eval.py 一致：约 99.9% 的诗词短于这个长度，
// 超长的多是元曲套数，截断只影响开头以外的内容。
const maxDocumentRunes = 1500

// Document 返回一首诗用于嵌入的文本：标题、作者、正文各占一行，正文段落直接相连。
// 规则与 eval/semantic/embed_eval.py 相同，评测结论才适用于发布的向量。
func Document(title, author string, paragraphs []string) string {
	text := title + "\n" + author + "\n" + strings.Join(paragraphs, "")
	if runes := []rune(text); len(runes) > maxDocumentRunes {
		return string(runes[:maxDocumentRunes])
	}
	return text
}

// TextHash 是 Document 文本的指纹，用来判断上一版的向量能否沿用。
type TextHash [16]byte

// HashText 计算文本的 TextHash（SHA-256 的前 16 字节）。
func HashText(text string) TextHash {
	sum := sha256.Sum256([]byte(text))
	var h TextHash
	copy(h[:], sum[:])
	return h
}

// Doc 是一首待嵌入的诗。
type Doc struct {
	ID   int64
	Text string
	Hash TextHash
}

// LoadCorpus 按 ID 升序读出主库中的全部诗词并拼成 Document。
// 简繁两表 ID 一一对应，向量只按简体生成一份，两种语言共用。
func LoadCorpus(db *database.DB) ([]Doc, error) {
	rows, err := db.Raw(fmt.Sprintf(`SELECT p.id, p.title, COALESCE(a.name, ''), p.content FROM %s p
		LEFT JOIN %s a ON a.id = p.author_id ORDER BY p.id`,
		database.PoemsTable(database.LangHans), database.AuthorsTable(database.LangHans))).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to read poems: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var docs []Doc
	for rows.Next() {
		var id int64
		var title, author, content string
		if err := rows.Scan(&id, &title, &author, &content); err != nil {
			return nil, err
		}
		var paragraphs []string
		if err := json.Unmarshal([]byte(content), &paragraphs); err != nil {
			return nil, fmt.Errorf("poem %d has malformed content: %w", id, err)
		}
		text := Document(title, author, paragraphs)
		docs = append(docs, Doc{ID: id, Text: text, Hash: HashText(text)})
	}
	return docs, rows.Err()
}

// Fingerprint 概括整份语料：每首诗的 ID 与文本哈希。向量文件记下生成时的指纹，
// 服务加载时重新计算并比对，主库换了版本（ID 或文本有变）就拒绝使用旧向量。
func Fingerprint(docs []Doc) string {
	h := sha256.New()
	var id [8]byte
	for _, d := range docs {
		binary.BigEndian.PutUint64(id[:], uint64(d.ID))
		h.Write(id[:])
		h.Write(d.Hash[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
