package store

import (
	"strings"
	"testing"

	"github.com/agynio/groups/migrations"
	"github.com/stretchr/testify/require"
)

func TestInitialMigrationDefinesGroupsModel(t *testing.T) {
	content, err := migrations.Files.ReadFile("0001_init.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, expected := range []string{
		"CREATE TYPE group_source AS ENUM ('platform', 'scim')",
		"CREATE TYPE group_member_type AS ENUM ('user', 'agent', 'app')",
		"CREATE TABLE groups",
		"CREATE TABLE group_memberships",
		"UNIQUE (organization_id, source, name)",
		"UNIQUE (group_id, member_id)",
		"REFERENCES groups(id) ON DELETE CASCADE",
	} {
		require.Contains(t, sql, expected)
	}
	require.False(t, strings.Contains(sql, "runner"))
}
