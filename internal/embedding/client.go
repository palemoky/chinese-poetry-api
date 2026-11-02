// Package embedding 调用 OpenAI 兼容的 /embeddings 接口（Ollama 与多数云服务都支持）把文本转成向量。
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client 是一个 OpenAI 兼容的嵌入接口。
type Client struct {
	// BaseURL 形如 http://localhost:11434/v1，请求发往 BaseURL + "/embeddings"。
	BaseURL string
	Model   string
	// APIKey 为空时不发送 Authorization 头（本地 Ollama 不需要）。
	APIKey string
	HTTP   *http.Client
	// Retries 是失败后的重试次数，只对网络错误、429 与 5xx 重试。
	Retries int
}

// New 返回一个带合理超时与重试的客户端。
func New(baseURL, model, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
		Retries: 4,
	}
}

type request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type response struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed 返回 texts 中每条文本的向量，顺序与 texts 相同。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(request{Model: c.Model, Input: texts})
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			}
		}
		vectors, retry, err := c.post(ctx, body, len(texts))
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) post(ctx context.Context, body []byte, n int) (vectors [][]float32, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, true, fmt.Errorf("failed to read embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retry, fmt.Errorf("embedding request returned %s: %s", resp.Status, truncate(string(data), 300))
	}

	var parsed response
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, false, fmt.Errorf("failed to parse embedding response: %w", err)
	}
	if len(parsed.Data) != n {
		return nil, false, fmt.Errorf("embedding response has %d vectors for %d inputs", len(parsed.Data), n)
	}
	vectors = make([][]float32, n)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= n || vectors[d.Index] != nil {
			return nil, false, errors.New("embedding response has invalid or duplicate indexes")
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, false, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
