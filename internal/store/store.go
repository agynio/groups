package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	groupColumns      = `id, organization_id, name, description, source, external_id, created_at, updated_at`
	membershipColumns = `id, group_id, member_type, member_id, source, created_at, updated_at`
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func scanGroup(row pgx.Row) (Group, error) {
	var (
		group      Group
		externalID pgtype.Text
	)
	if err := row.Scan(
		&group.Meta.ID,
		&group.OrganizationID,
		&group.Name,
		&group.Description,
		&group.Source,
		&externalID,
		&group.Meta.CreatedAt,
		&group.Meta.UpdatedAt,
	); err != nil {
		return Group{}, err
	}
	if externalID.Valid {
		group.ExternalID = &externalID.String
	}
	return group, nil
}

func scanMembership(row pgx.Row) (GroupMembership, error) {
	var membership GroupMembership
	if err := row.Scan(
		&membership.Meta.ID,
		&membership.GroupID,
		&membership.MemberType,
		&membership.MemberID,
		&membership.Source,
		&membership.Meta.CreatedAt,
		&membership.Meta.UpdatedAt,
	); err != nil {
		return GroupMembership{}, err
	}
	return membership, nil
}

func (s *Store) CreateGroup(ctx context.Context, input CreateGroupInput) (Group, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO groups (id, organization_id, name, description, source, external_id)
             VALUES ($1, $2, $3, $4, $5, $6)
             RETURNING %s`, groupColumns),
		input.ID,
		input.OrganizationID,
		input.Name,
		input.Description,
		input.Source,
		input.ExternalID,
	)
	group, err := scanGroup(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Group{}, AlreadyExists("group")
		}
		return Group{}, err
	}
	return group, nil
}

func (s *Store) GetGroup(ctx context.Context, id uuid.UUID) (Group, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM groups WHERE id = $1`, groupColumns),
		id,
	)
	group, err := scanGroup(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Group{}, NotFound("group")
		}
		return Group{}, err
	}
	return group, nil
}

func (s *Store) ListGroups(ctx context.Context, filter ListGroupsFilter, pageSize int32, cursor *PageCursor) ([]Group, *PageCursor, error) {
	clauses := []string{"organization_id = $1"}
	args := []any{filter.OrganizationID}
	if filter.Source != nil {
		clauses, args = appendClause(clauses, args, "source = $%d", *filter.Source)
	}

	groups, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM groups", groupColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanGroup,
		func(group Group) uuid.UUID { return group.Meta.ID },
	)
	if err != nil {
		return nil, nil, err
	}
	return groups, nextCursor, nil
}

func (s *Store) UpdateGroup(ctx context.Context, input UpdateGroupInput) (Group, error) {
	builder := updateBuilder{}
	if input.Name != nil {
		builder.add("name", *input.Name)
	}
	if input.Description != nil {
		builder.add("description", *input.Description)
	}
	if builder.empty() {
		return s.GetGroup(ctx, input.ID)
	}
	query, args := builder.build("groups", groupColumns, input.ID)
	group, err := scanGroup(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Group{}, NotFound("group")
		}
		if isUniqueViolation(err) {
			return Group{}, AlreadyExists("group")
		}
		return Group{}, err
	}
	return group, nil
}

func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) (DeletedGroup, error) {
	var deleted DeletedGroup
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		group, err := scanGroup(tx.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM groups WHERE id = $1`, groupColumns), id))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFound("group")
			}
			return err
		}

		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM group_memberships WHERE group_id = $1`, membershipColumns), id)
		if err != nil {
			return err
		}
		defer rows.Close()

		memberships := []GroupMembership{}
		for rows.Next() {
			membership, err := scanMembership(rows)
			if err != nil {
				return err
			}
			memberships = append(memberships, membership)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id); err != nil {
			return err
		}
		deleted = DeletedGroup{Group: group, Memberships: memberships}
		return nil
	})
	if err != nil {
		return DeletedGroup{}, err
	}
	return deleted, nil
}

func (s *Store) AddMember(ctx context.Context, input AddMemberInput) (GroupMembership, bool, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO group_memberships (id, group_id, member_type, member_id, source)
             VALUES ($1, $2, $3, $4, $5)
             ON CONFLICT (group_id, member_id) DO NOTHING
             RETURNING %s`, membershipColumns),
		input.ID,
		input.GroupID,
		input.MemberType,
		input.MemberID,
		input.Source,
	)
	membership, err := scanMembership(row)
	if err == nil {
		return membership, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return GroupMembership{}, false, err
	}
	membership, err = s.GetMembershipByGroupMember(ctx, input.GroupID, input.MemberID)
	if err != nil {
		return GroupMembership{}, false, err
	}
	return membership, false, nil
}

func (s *Store) GetMembershipByGroupMember(ctx context.Context, groupID uuid.UUID, memberID uuid.UUID) (GroupMembership, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM group_memberships WHERE group_id = $1 AND member_id = $2`, membershipColumns),
		groupID,
		memberID,
	)
	membership, err := scanMembership(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GroupMembership{}, NotFound("membership")
		}
		return GroupMembership{}, err
	}
	return membership, nil
}

func (s *Store) RemoveMember(ctx context.Context, groupID uuid.UUID, memberID uuid.UUID) (RemovedMembership, bool, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`DELETE FROM group_memberships
             USING groups
             WHERE group_memberships.group_id = groups.id
               AND group_memberships.group_id = $1
               AND group_memberships.member_id = $2
             RETURNING %s, groups.organization_id`, qualifiedMembershipColumns("group_memberships")),
		groupID,
		memberID,
	)
	var removed RemovedMembership
	membership, err := scanRemovedMembership(row, &removed.OrganizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RemovedMembership{}, false, nil
		}
		return RemovedMembership{}, false, err
	}
	removed.Membership = membership
	return removed, true, nil
}

func (s *Store) ListMembers(ctx context.Context, filter ListMembersFilter, pageSize int32, cursor *PageCursor) ([]GroupMembership, *PageCursor, error) {
	clauses := []string{"group_id = $1"}
	args := []any{filter.GroupID}
	if filter.MemberType != nil {
		clauses, args = appendClause(clauses, args, "member_type = $%d", *filter.MemberType)
	}

	memberships, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM group_memberships", membershipColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanMembership,
		func(membership GroupMembership) uuid.UUID { return membership.Meta.ID },
	)
	if err != nil {
		return nil, nil, err
	}
	return memberships, nextCursor, nil
}

func (s *Store) ListMemberGroups(ctx context.Context, filter ListMemberGroupsFilter, pageSize int32, cursor *PageCursor) ([]Group, *PageCursor, error) {
	clauses := []string{
		"group_memberships.member_type = $1",
		"group_memberships.member_id = $2",
		"groups.organization_id = $3",
	}
	args := []any{filter.MemberType, filter.MemberID, filter.OrganizationID}
	groups, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf(`SELECT %s FROM groups JOIN group_memberships ON groups.id = group_memberships.group_id`, qualifiedGroupColumns("groups")),
		clauses,
		args,
		cursor,
		pageSize,
		scanGroup,
		func(group Group) uuid.UUID { return group.Meta.ID },
	)
	if err != nil {
		return nil, nil, err
	}
	return groups, nextCursor, nil
}

func qualifiedGroupColumns(table string) string {
	return fmt.Sprintf(`%s.id, %s.organization_id, %s.name, %s.description, %s.source, %s.external_id, %s.created_at, %s.updated_at`, table, table, table, table, table, table, table, table)
}

func qualifiedMembershipColumns(table string) string {
	return fmt.Sprintf(`%s.id, %s.group_id, %s.member_type, %s.member_id, %s.source, %s.created_at, %s.updated_at`, table, table, table, table, table, table, table)
}

func scanRemovedMembership(row pgx.Row, organizationID *uuid.UUID) (GroupMembership, error) {
	var membership GroupMembership
	if err := row.Scan(
		&membership.Meta.ID,
		&membership.GroupID,
		&membership.MemberType,
		&membership.MemberID,
		&membership.Source,
		&membership.Meta.CreatedAt,
		&membership.Meta.UpdatedAt,
		organizationID,
	); err != nil {
		return GroupMembership{}, err
	}
	return membership, nil
}
