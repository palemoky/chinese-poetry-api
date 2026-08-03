package database

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// 游标（cursor）分页用的编解码。
//
// 诗词列表固定按 id 升序排列，因此「上一页的最后一条 id」就足以定位下一页的起点：
// WHERE id > ? ORDER BY id LIMIT n 是一次索引区间扫描，代价与翻到第几页无关，
// 而 LIMIT/OFFSET 必须逐行读出并丢弃前 N 行，代价与页码成正比
// （30 万首语料上 offset=300000 约 158ms，offset=0 约 0.6ms）。
//
// 游标对外是不透明字符串：编码方式（含 poem: 前缀与 base64）属于实现细节，
// 客户端只应原样回传。加前缀是为了让「把一个作者游标传给诗词接口」这类误用
// 在解码时就被拒绝，而不是被当成一个碰巧合法的 id 静默接受。

const cursorPrefixPoem = "poem"

// EncodePoemCursor 把诗词 id 编码为不透明游标。
func EncodePoemCursor(id int64) string {
	return encodeCursor(cursorPrefixPoem, id)
}

// DecodePoemCursor 把游标解码回诗词 id。
func DecodePoemCursor(cursor string) (int64, error) {
	return decodeCursor(cursorPrefixPoem, cursor)
}

func encodeCursor(prefix string, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(prefix + ":" + strconv.FormatInt(id, 10)))
}

func decodeCursor(prefix, cursor string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("malformed cursor")
	}

	value, ok := strings.CutPrefix(string(raw), prefix+":")
	if !ok {
		return 0, fmt.Errorf("malformed cursor")
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed cursor")
	}
	return id, nil
}
