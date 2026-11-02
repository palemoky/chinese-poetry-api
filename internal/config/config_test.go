package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, defaultTrustedProxies, cfg.Server.TrustedProxies)
	assert.Equal(t, 5000, cfg.GraphQL.ComplexityLimit)
	assert.True(t, cfg.GraphQL.Introspection)
	assert.Equal(t, "data/poetry.db", cfg.Database.Path)
}

// database.path 曾在读完配置文件后被强行改回 data/poetry.db，配置文件里写了也不生效
func TestLoadDatabasePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("database:\n  path: /srv/poetry.db\n"), 0o600))

	cfg, err := Load(file)
	require.NoError(t, err)
	assert.Equal(t, "/srv/poetry.db", cfg.Database.Path)

	t.Setenv("DB_PATH", "/mnt/poetry.db")
	cfg, err = Load(file)
	require.NoError(t, err)
	assert.Equal(t, "/mnt/poetry.db", cfg.Database.Path, "the environment overrides the file")
}

func TestLoadTrustedProxiesFromEnv(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 10.0.0.1, 192.168.0.0/16 ,")
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "192.168.0.0/16"}, cfg.Server.TrustedProxies)

	t.Setenv("TRUSTED_PROXIES", "none")
	cfg, err = Load("")
	require.NoError(t, err)
	assert.Empty(t, cfg.Server.TrustedProxies)
}

func TestLoadRejectsInvalidTrustedProxy(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "not-an-ip")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-an-ip")
}

func TestLoadConfigFile(t *testing.T) {
	cfg, err := Load("../../config.yaml")
	require.NoError(t, err)
	assert.Equal(t, 5000, cfg.GraphQL.ComplexityLimit)
	assert.Equal(t, defaultTrustedProxies, cfg.Server.TrustedProxies)
}

// config.yaml 的 semantic 段全是注释，解析为 null。此前设了 EMBEDDING_API_URL 后
// vectors_path 的默认值会丢失，Docker 里配置了嵌入接口也开不了语义检索。
func TestLoadConfigFileKeepsDefaultsInNullSection(t *testing.T) {
	t.Setenv("EMBEDDING_API_URL", "http://ollama:11434/v1")
	cfg, err := Load("../../config.yaml")
	require.NoError(t, err)
	assert.Equal(t, SemanticConfig{
		VectorsPath: "data/poetry-vectors.db", EmbeddingURL: "http://ollama:11434/v1",
	}, cfg.Semantic)

	// 嵌套的 null 也一样处理
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: 9000\n  trusted_proxies:\nsemantic:\n"), 0o600))
	t.Setenv("PORT", "")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1")
	cfg, err = Load(path)
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, []string{"10.0.0.1"}, cfg.Server.TrustedProxies)
	assert.Equal(t, "data/poetry-vectors.db", cfg.Semantic.VectorsPath)
}

func TestLoadSemanticFromEnv(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "data/poetry-vectors.db", cfg.Semantic.VectorsPath)
	assert.Empty(t, cfg.Semantic.EmbeddingURL, "off unless configured")

	t.Setenv("VECTORS_PATH", "/data/v.db")
	t.Setenv("EMBEDDING_API_URL", "http://ollama:11434/v1")
	t.Setenv("EMBEDDING_MODEL", "BAAI/bge-m3")
	cfg, err = Load("")
	require.NoError(t, err)
	assert.Equal(t, SemanticConfig{
		VectorsPath: "/data/v.db", EmbeddingURL: "http://ollama:11434/v1", EmbeddingModel: "BAAI/bge-m3",
	}, cfg.Semantic)
}
