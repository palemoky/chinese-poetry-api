package config

import (
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
