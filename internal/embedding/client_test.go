package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedOrdersByIndexAndSendsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		var req request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "bge-m3", req.Model)
		assert.Equal(t, []string{"a", "b"}, req.Input)
		// 故意倒序返回，客户端必须按 index 归位
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[2,2]},{"index":0,"embedding":[1,1]}]}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL+"/v1/", "bge-m3", "secret").Embed(context.Background(), []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1, 1}, {2, 2}}, got)
}

func TestEmbedRetriesServerErrorsOnly(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		assert.Empty(t, r.Header.Get("Authorization"), "no key, no header")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}]}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "m", "").Embed(context.Background(), []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1}}, got)
	assert.EqualValues(t, 2, calls.Load())

	calls.Store(0)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unknown model", http.StatusBadRequest)
	}))
	defer bad.Close()
	_, err = New(bad.URL, "m", "").Embed(context.Background(), []string{"a"})
	assert.ErrorContains(t, err, "unknown model")
	assert.EqualValues(t, 1, calls.Load(), "client errors are not retried")
}

func TestEmbedRejectsMalformedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"too few vectors":    `{"data":[{"index":0,"embedding":[1]}]}`,
		"duplicate indexes":  `{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`,
		"index out of range": `{"data":[{"index":0,"embedding":[1]},{"index":5,"embedding":[2]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			_, err := New(srv.URL, "m", "").Embed(context.Background(), []string{"a", "b"})
			assert.Error(t, err)
		})
	}
}

func TestEmbedHonorsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var first time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "2")
			http.Error(w, "TPM limit reached", http.StatusTooManyRequests)
			return
		}
		assert.GreaterOrEqual(t, time.Since(first), 2*time.Second, "waited as long as the server asked")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}]}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "m", "").Embed(context.Background(), []string{"a"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, calls.Load())
}

func TestBackoffIsCapped(t *testing.T) {
	c := &Client{MaxBackoff: 5 * time.Second}
	assert.Equal(t, time.Second, c.backoff(1))
	assert.Equal(t, 4*time.Second, c.backoff(3))
	assert.Equal(t, 5*time.Second, c.backoff(10))
	assert.Equal(t, 5*time.Second, c.backoff(100), "no overflow on long runs")
}
