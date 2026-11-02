package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// 「唐诗」体裁本身没有作品，作品都在它所属分类下的五言绝句等体裁里，
// 按「唐诗」过滤曾经什么也查不到（#120）。
func TestCategoryTypeFilterCoversWholeCategory(t *testing.T) {
	repo := NewRepository(setupTestDB(t))

	types := []PoetryType{
		{ID: 10, Name: "唐诗", Category: "唐诗"},
		{ID: 11, Name: "五言绝句", Category: "唐诗"},
		{ID: 14, Name: "七言律诗", Category: "唐诗"},
		{ID: 20, Name: "宋词", Category: "宋词"},
		{ID: 99, Name: "其他", Category: "其他"},
	}
	for i := range types {
		require.NoError(t, createTestPoetryType(repo, &types[i]))
	}
	for i, typeID := range []int64{11, 14, 20, 99} {
		require.NoError(t, createTestPoem(repo, &Poem{
			ID:      int64(i + 1),
			Title:   "诗" + string(rune('一'+i)),
			Content: datatypes.JSON(`["床前明月光"]`),
			TypeID:  &typeID,
		}))
	}

	typeIDsOf := func(poems []Poem) []int64 {
		var ids []int64
		for _, p := range poems {
			ids = append(ids, *p.TypeID)
		}
		return ids
	}

	tests := []struct {
		name    string
		typeIDs []int64
		want    []int64
	}{
		{"a category type covers its category", []int64{10}, []int64{11, 14}},
		{"a concrete type is not expanded", []int64{11}, []int64{11}},
		{"a category type with no subtypes stays itself", []int64{20}, []int64{20}},
		{"mixed with another type", []int64{10, 20}, []int64{11, 14, 20}},
		{"an unknown type still matches nothing", []int64{12345}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poems, total, err := repo.ListPoemsWithFilter(10, 0, nil, nil, tt.typeIDs)
			require.NoError(t, err)
			assert.Equal(t, tt.want, typeIDsOf(poems))
			assert.Equal(t, len(tt.want), total)

			poems, _, err = repo.ListPoemsAfter(10, nil, nil, nil, tt.typeIDs)
			require.NoError(t, err)
			assert.Equal(t, tt.want, typeIDsOf(poems))

			poem, err := repo.GetRandomPoem(nil, nil, tt.typeIDs)
			if tt.want == nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, tt.want, *poem.TypeID)
		})
	}
}
