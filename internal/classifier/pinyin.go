package classifier

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

var pinyinArgs = pinyin.NewArgs()

func init() {
	// Use tone marks for full pinyin
	pinyinArgs.Style = pinyin.Tone
	pinyinArgs.Heteronym = false
}

// ToPinyin converts Chinese text to pinyin with tone marks
func ToPinyin(text string) string {
	if text == "" {
		return ""
	}

	result := pinyin.Pinyin(text, pinyinArgs)
	var parts []string
	for _, item := range result {
		if len(item) > 0 {
			parts = append(parts, item[0])
		}
	}

	return strings.Join(parts, " ")
}

// ToPinyinNoTone converts Chinese text to pinyin without tone marks
func ToPinyinNoTone(text string) string {
	if text == "" {
		return ""
	}

	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	args.Heteronym = false

	result := pinyin.Pinyin(text, args)
	var parts []string
	for _, item := range result {
		if len(item) > 0 {
			parts = append(parts, item[0])
		}
	}

	return strings.Join(parts, " ")
}

// ToPinyinAbbr converts Chinese text to pinyin abbreviation (first letters)
func ToPinyinAbbr(text string) string {
	if text == "" {
		return ""
	}

	args := pinyin.NewArgs()
	args.Style = pinyin.FirstLetter
	args.Heteronym = false

	result := pinyin.Pinyin(text, args)
	var parts []string
	for _, item := range result {
		if len(item) > 0 {
			parts = append(parts, item[0])
		}
	}

	return strings.Join(parts, "")
}
