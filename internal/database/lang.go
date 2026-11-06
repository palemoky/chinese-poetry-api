package database

import (
	"fmt"
	"io"
	"strconv"
)

// Lang 表示中文的简繁语言变体。
type Lang string

const (
	// LangHans 表示简体中文
	LangHans Lang = "zh-Hans"
	// LangHant 表示繁体中文
	LangHant Lang = "zh-Hant"
)

// IsValid 判断该语言变体是否合法。
func (l Lang) IsValid() bool {
	return l == LangHans || l == LangHant
}

// Default 返回自身，非法值则回落到默认的简体中文。
func (l Lang) Default() Lang {
	if l.IsValid() {
		return l
	}
	return LangHans
}

// langAliases 收录语言变体所有可接受的写法及其对应的 Lang。
var langAliases = map[string]Lang{
	"zh-Hans":    LangHans,
	"zh_Hans":    LangHans,
	"hans":       LangHans,
	"sc":         LangHans,
	"simplified": LangHans,

	"zh-Hant":     LangHant,
	"zh_Hant":     LangHant,
	"hant":        LangHant,
	"tc":          LangHant,
	"traditional": LangHant,
}

// LookupLang 把字符串解析为 Lang，并返回 s 是否为已知写法，
// 便于调用方直接拒绝拼写错误，而不是悄悄按 zh-Hans 处理。
func LookupLang(s string) (Lang, bool) {
	lang, ok := langAliases[s]
	return lang, ok
}

// Lang 在 GraphQL 中的枚举名，与 schema.graphqls 里 Lang 枚举的声明一致。
const (
	gqlLangHans = "ZH_HANS"
	gqlLangHant = "ZH_HANT"
)

// UnmarshalGQL 实现 graphql.Unmarshaler，把 schema 中的 Lang 枚举映射到本类型。
//
// 若不实现，gqlgen 的 autobind 会把枚举名直接转成 Lang，得到 Lang("ZH_HANT")，
// 这个值既不等于 LangHans 也不等于 LangHant，导致所有表名辅助函数都落到简体分支，
// lang 参数在整个 GraphQL API 中形同虚设。
func (l *Lang) UnmarshalGQL(v any) error {
	name, ok := v.(string)
	if !ok {
		return fmt.Errorf("Lang must be one of %s or %s, got %T", gqlLangHans, gqlLangHant, v)
	}

	switch name {
	case gqlLangHans:
		*l = LangHans
	case gqlLangHant:
		*l = LangHant
	default:
		return fmt.Errorf("Lang must be one of %s or %s, got %q", gqlLangHans, gqlLangHant, name)
	}
	return nil
}

// MarshalGQL 实现 graphql.Marshaler，输出的是枚举名而非底层的
// "zh-Hans"/"zh-Hant" 取值，后者并非合法的枚举字面量。
func (l Lang) MarshalGQL(w io.Writer) {
	name := gqlLangHans
	if l == LangHant {
		name = gqlLangHant
	}
	// 该接口没有 error 返回值，且 gqlgen 的 writer 会把写入失败记录在响应上，
	// 因此这里对写入错误无事可做。
	_, _ = io.WriteString(w, strconv.Quote(name))
}

// 以下辅助函数用于拼接带语言后缀的表名。

// PoemsTable 返回指定语言变体的诗词表名。
func PoemsTable(lang Lang) string {
	if lang == LangHant {
		return "poems_zh_hant"
	}
	return "poems_zh_hans"
}

// AuthorsTable 返回指定语言变体的作者表名。
func AuthorsTable(lang Lang) string {
	if lang == LangHant {
		return "authors_zh_hant"
	}
	return "authors_zh_hans"
}

// DynastiesTable 返回指定语言变体的朝代表名。
func DynastiesTable(lang Lang) string {
	if lang == LangHant {
		return "dynasties_zh_hant"
	}
	return "dynasties_zh_hans"
}

// PoetryTypesTable 返回指定语言变体的体裁表名。
func PoetryTypesTable(lang Lang) string {
	if lang == LangHant {
		return "poetry_types_zh_hant"
	}
	return "poetry_types_zh_hans"
}

// PoemsFtsTable 返回指定语言变体诗词表所对应的 FTS5 全文检索虚拟表名。
func PoemsFtsTable(lang Lang) string {
	if lang == LangHant {
		return "poems_fts_zh_hant"
	}
	return "poems_fts_zh_hans"
}

// 包内使用的小写版本
func poemsTable(lang Lang) string       { return PoemsTable(lang) }
func authorsTable(lang Lang) string     { return AuthorsTable(lang) }
func dynastiesTable(lang Lang) string   { return DynastiesTable(lang) }
func poetryTypesTable(lang Lang) string { return PoetryTypesTable(lang) }
func poemsFtsTable(lang Lang) string    { return PoemsFtsTable(lang) }
