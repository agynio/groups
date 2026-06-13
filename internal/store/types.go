package store

import (
	"time"

	"github.com/google/uuid"
)

type EntityMeta struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GroupSource string

const (
	GroupSourcePlatform GroupSource = "platform"
	GroupSourceSCIM     GroupSource = "scim"
)

type GroupMemberType string

const (
	GroupMemberTypeUser  GroupMemberType = "user"
	GroupMemberTypeAgent GroupMemberType = "agent"
	GroupMemberTypeApp   GroupMemberType = "app"
)

type Group struct {
	Meta           EntityMeta
	OrganizationID uuid.UUID
	Name           string
	Description    string
	Source         GroupSource
	ExternalID     *string
}

type GroupMembership struct {
	Meta       EntityMeta
	GroupID    uuid.UUID
	MemberType GroupMemberType
	MemberID   uuid.UUID
	Source     GroupSource
}

type CreateGroupInput struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Description    string
	Source         GroupSource
	ExternalID     *string
}

type UpdateGroupInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
}

type ListGroupsFilter struct {
	OrganizationID uuid.UUID
	Source         *GroupSource
}

type AddMemberInput struct {
	ID         uuid.UUID
	GroupID    uuid.UUID
	MemberType GroupMemberType
	MemberID   uuid.UUID
	Source     GroupSource
}

type DeletedGroup struct {
	Group       Group
	Memberships []GroupMembership
	Admins      []uuid.UUID
}

type RemovedMembership struct {
	Membership     GroupMembership
	OrganizationID uuid.UUID
}

type ListMembersFilter struct {
	GroupID    uuid.UUID
	MemberType *GroupMemberType
}

type ListMemberGroupsFilter struct {
	MemberType     GroupMemberType
	MemberID       uuid.UUID
	OrganizationID uuid.UUID
}
