package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFromEnvSetsReconciliationInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://groups")
	t.Setenv("RECONCILIATION_INTERVAL", "30s")

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, cfg.ReconciliationInterval)
}

func TestFromEnvAcceptsReconciliationIntervalSeconds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://groups")
	t.Setenv("RECONCILIATION_INTERVAL", "45")

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, 45*time.Second, cfg.ReconciliationInterval)
}

func TestFromEnvRejectsInvalidReconciliationInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://groups")
	t.Setenv("RECONCILIATION_INTERVAL", "soon")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "RECONCILIATION_INTERVAL")
}
