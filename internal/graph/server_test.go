package graph

import (
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nestedQuery 是一次请求放大成大量查询的典型写法：
// 每个作者展开其诗作，每首诗再展开作者及其诗作。
const nestedQuery = `{
  authors(pageSize: 100) {
    edges { node { poems(pageSize: 100) {
      edges { node { author { poems(pageSize: 100) {
        edges { node { title } }
      } } } }
    } } }
  }
}`

func TestServerRejectsOverlyComplexQuery(t *testing.T) {
	resolver, _ := setupTestResolver(t)
	c := client.New(NewServer(resolver, ServerOptions{ComplexityLimit: 5000}))

	var resp map[string]any
	err := c.Post(nestedQuery, &resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "complexity")
}

func TestServerAllowsOrdinaryPagedQuery(t *testing.T) {
	resolver, repo := setupTestResolver(t)
	createTestData(t, repo)
	c := client.New(NewServer(resolver, ServerOptions{ComplexityLimit: 5000}))

	// 单层满页查询是正常用法，不应被复杂度上限拦下
	var resp struct {
		Poems struct {
			TotalCount int
			Edges      []any
		}
	}
	err := c.Post(`{ poems(pageSize: 100) {
	  totalCount
	  edges { node { id title content author { name } dynasty { name } } }
	} }`, &resp)
	require.NoError(t, err)
	assert.Positive(t, resp.Poems.TotalCount)
}

func TestServerIntrospectionToggle(t *testing.T) {
	resolver, _ := setupTestResolver(t)
	const query = `{ __schema { queryType { name } } }`

	var resp map[string]any
	off := client.New(NewServer(resolver, ServerOptions{ComplexityLimit: 5000}))
	require.Error(t, off.Post(query, &resp), "introspection should be rejected when disabled")

	on := client.New(NewServer(resolver, ServerOptions{ComplexityLimit: 5000, Introspection: true}))
	require.NoError(t, on.Post(query, &resp))
}

func TestPagedComplexity(t *testing.T) {
	ptr := func(n int) *int { return &n }

	assert.Equal(t, 1+3*defaultPageSize, pagedComplexity(3, nil), "missing pageSize uses the default")
	assert.Equal(t, 1+3*10, pagedComplexity(3, ptr(10)))
	assert.Equal(t, 1+3*maxPageSize, pagedComplexity(3, ptr(100000)), "oversized pageSize is capped")
	assert.Equal(t, 1+3, pagedComplexity(3, ptr(-5)), "non-positive pageSize counts as one")
}
