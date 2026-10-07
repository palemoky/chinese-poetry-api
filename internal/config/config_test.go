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
