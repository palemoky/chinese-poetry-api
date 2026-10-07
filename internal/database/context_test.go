package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryQueriesHonorContext(t *testing.T) {
	repo := NewRepository(setupTestDB(t))
	_, err := repo.CountPoems()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// WithLang 不应丢掉已绑定的上下文
	_, _, err = repo.WithContext(ctx).WithLang(LangHans).ListPoemsWithFilter(10, 0, nil, nil, nil)
	assert.ErrorIs(t, err, context.Canceled)

	// 原实例不受影响
	_, _, err = repo.ListPoemsWithFilter(10, 0, nil, nil, nil)
	assert.NoError(t, err)
}

// 截止时间到了，正在执行的查询要被中断，而不是跑完才返回
func TestRepositoryDeadlineInterruptsRunningQuery(t *testing.T) {
	repo := NewRepository(setupTestDB(t))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	var n int64
	err := repo.WithContext(ctx).conn().
		Raw("WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c").
		Scan(&n).Error
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
