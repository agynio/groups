package server

import (
	"context"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/events"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
)

type GroupStore interface {
	CreateGroup(ctx context.Context, input store.CreateGroupInput) (store.Group, error)
	GetGroup(ctx context.Context, id uuid.UUID) (store.Group, error)
	ListGroups(ctx context.Context, filter store.ListGroupsFilter, pageSize int32, cursor *store.PageCursor) ([]store.Group, *store.PageCursor, error)
	UpdateGroup(ctx context.Context, input store.UpdateGroupInput) (store.Group, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) (store.DeletedGroup, error)
	AddMember(ctx context.Context, input store.AddMemberInput) (store.GroupMembership, bool, error)
	RemoveMember(ctx context.Context, groupID uuid.UUID, memberID uuid.UUID) (store.RemovedMembership, bool, error)
	ListMembers(ctx context.Context, filter store.ListMembersFilter, pageSize int32, cursor *store.PageCursor) ([]store.GroupMembership, *store.PageCursor, error)
	ListMemberGroups(ctx context.Context, filter store.ListMemberGroupsFilter, pageSize int32, cursor *store.PageCursor) ([]store.Group, *store.PageCursor, error)
}

type Server struct {
	groupsv1.UnimplementedGroupsServiceServer
	store               GroupStore
	authorizationClient authorizationv1.AuthorizationServiceClient
	identityClient      identityv1.IdentityServiceClient
	publisher           events.Publisher
}

func New(
	store GroupStore,
	authorizationClient authorizationv1.AuthorizationServiceClient,
	identityClient identityv1.IdentityServiceClient,
	publisher events.Publisher,
) *Server {
	return &Server{
		store:               store,
		authorizationClient: authorizationClient,
		identityClient:      identityClient,
		publisher:           publisher,
	}
}
