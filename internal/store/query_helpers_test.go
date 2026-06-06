package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildListMemberGroupsQueryUsesQualifiedCursorColumn(t *testing.T) {
	cursorID := uuid.New()
	query, args, limit := buildListEntitiesQuery(
		"SELECT groups.id FROM groups JOIN group_memberships ON groups.id = group_memberships.group_id",
		[]string{
			"group_memberships.member_type = $1",
			"group_memberships.member_id = $2",
			"groups.organization_id = $3",
		},
		[]any{GroupMemberTypeUser, uuid.New(), uuid.New()},
		&PageCursor{AfterID: cursorID},
		2,
		"groups.id",
	)

	require.Contains(t, query, "groups.id > $4")
	require.Contains(t, query, "ORDER BY groups.id ASC LIMIT $5")
	require.NotContains(t, query, " id >")
	require.Equal(t, int32(2), limit)
	require.Len(t, args, 5)
	require.Equal(t, cursorID, args[3])
	require.Equal(t, 3, args[4])
}
