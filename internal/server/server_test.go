package server

import (
	"context"
	"errors"
	"testing"
	"time"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeStore struct {
	groups      map[uuid.UUID]store.Group
	memberships map[uuid.UUID]store.GroupMembership
	deleted     []uuid.UUID
	now         time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		groups:      map[uuid.UUID]store.Group{},
		memberships: map[uuid.UUID]store.GroupMembership{},
		now:         time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC),
	}
}

func (s *fakeStore) CreateGroup(ctx context.Context, input store.CreateGroupInput) (store.Group, error) {
	for _, group := range s.groups {
		if group.OrganizationID == input.OrganizationID && group.Source == input.Source && group.Name == input.Name {
			return store.Group{}, store.AlreadyExists("group")
		}
	}
	group := store.Group{
		Meta: store.EntityMeta{
			ID:        input.ID,
			CreatedAt: s.now,
			UpdatedAt: s.now,
		},
		OrganizationID: input.OrganizationID,
		Name:           input.Name,
		Description:    input.Description,
		Source:         input.Source,
		ExternalID:     input.ExternalID,
	}
	s.groups[group.Meta.ID] = group
	s.now = s.now.Add(time.Minute)
	return group, nil
}

func (s *fakeStore) GetGroup(ctx context.Context, id uuid.UUID) (store.Group, error) {
	group, ok := s.groups[id]
	if !ok {
		return store.Group{}, store.NotFound("group")
	}
	return group, nil
}

func (s *fakeStore) ListGroups(ctx context.Context, filter store.ListGroupsFilter, pageSize int32, cursor *store.PageCursor) ([]store.Group, *store.PageCursor, error) {
	groups := []store.Group{}
	for _, group := range s.groups {
		if group.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.Source != nil && group.Source != *filter.Source {
			continue
		}
		groups = append(groups, group)
	}
	return groups, nil, nil
}

func (s *fakeStore) UpdateGroup(ctx context.Context, input store.UpdateGroupInput) (store.Group, error) {
	group, ok := s.groups[input.ID]
	if !ok {
		return store.Group{}, store.NotFound("group")
	}
	if input.Name != nil {
		group.Name = *input.Name
	}
	if input.Description != nil {
		group.Description = *input.Description
	}
	group.Meta.UpdatedAt = s.now
	s.groups[group.Meta.ID] = group
	s.now = s.now.Add(time.Minute)
	return group, nil
}

func (s *fakeStore) DeleteGroup(ctx context.Context, id uuid.UUID) (store.DeletedGroup, error) {
	group, ok := s.groups[id]
	if !ok {
		return store.DeletedGroup{}, store.NotFound("group")
	}
	deletedMemberships := []store.GroupMembership{}
	delete(s.groups, id)
	s.deleted = append(s.deleted, id)
	for membershipID, membership := range s.memberships {
		if membership.GroupID == id {
			deletedMemberships = append(deletedMemberships, membership)
			delete(s.memberships, membershipID)
		}
	}
	return store.DeletedGroup{Group: group, Memberships: deletedMemberships}, nil
}

func (s *fakeStore) AddMember(ctx context.Context, input store.AddMemberInput) (store.GroupMembership, bool, error) {
	for _, membership := range s.memberships {
		if membership.GroupID == input.GroupID && membership.MemberID == input.MemberID {
			return membership, false, nil
		}
	}
	membership := store.GroupMembership{
		Meta: store.EntityMeta{
			ID:        input.ID,
			CreatedAt: s.now,
			UpdatedAt: s.now,
		},
		GroupID:    input.GroupID,
		MemberType: input.MemberType,
		MemberID:   input.MemberID,
		Source:     input.Source,
	}
	s.memberships[membership.Meta.ID] = membership
	s.now = s.now.Add(time.Minute)
	return membership, true, nil
}

func (s *fakeStore) RemoveMember(ctx context.Context, groupID uuid.UUID, memberID uuid.UUID) (store.RemovedMembership, bool, error) {
	for membershipID, membership := range s.memberships {
		if membership.GroupID == groupID && membership.MemberID == memberID {
			delete(s.memberships, membershipID)
			group := s.groups[groupID]
			return store.RemovedMembership{Membership: membership, OrganizationID: group.OrganizationID}, true, nil
		}
	}
	return store.RemovedMembership{}, false, nil
}

func (s *fakeStore) ListMembers(ctx context.Context, filter store.ListMembersFilter, pageSize int32, cursor *store.PageCursor) ([]store.GroupMembership, *store.PageCursor, error) {
	memberships := []store.GroupMembership{}
	for _, membership := range s.memberships {
		if membership.GroupID != filter.GroupID {
			continue
		}
		if filter.MemberType != nil && membership.MemberType != *filter.MemberType {
			continue
		}
		memberships = append(memberships, membership)
	}
	return memberships, nil, nil
}

func (s *fakeStore) ListMemberGroups(ctx context.Context, filter store.ListMemberGroupsFilter, pageSize int32, cursor *store.PageCursor) ([]store.Group, *store.PageCursor, error) {
	groups := []store.Group{}
	for _, membership := range s.memberships {
		if membership.MemberType != filter.MemberType || membership.MemberID != filter.MemberID {
			continue
		}
		group := s.groups[membership.GroupID]
		if group.OrganizationID == filter.OrganizationID {
			groups = append(groups, group)
		}
	}
	return groups, nil, nil
}

type fakeAuthorizationClient struct {
	writes []*authorizationv1.WriteRequest
	err    error
}

func (c *fakeAuthorizationClient) Check(context.Context, *authorizationv1.CheckRequest, ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	return &authorizationv1.CheckResponse{Allowed: true}, nil
}

func (c *fakeAuthorizationClient) BatchCheck(context.Context, *authorizationv1.BatchCheckRequest, ...grpc.CallOption) (*authorizationv1.BatchCheckResponse, error) {
	return &authorizationv1.BatchCheckResponse{}, nil
}

func (c *fakeAuthorizationClient) Write(ctx context.Context, req *authorizationv1.WriteRequest, opts ...grpc.CallOption) (*authorizationv1.WriteResponse, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.writes = append(c.writes, req)
	return &authorizationv1.WriteResponse{}, nil
}

func (c *fakeAuthorizationClient) Read(context.Context, *authorizationv1.ReadRequest, ...grpc.CallOption) (*authorizationv1.ReadResponse, error) {
	return &authorizationv1.ReadResponse{}, nil
}

func (c *fakeAuthorizationClient) ListObjects(context.Context, *authorizationv1.ListObjectsRequest, ...grpc.CallOption) (*authorizationv1.ListObjectsResponse, error) {
	return &authorizationv1.ListObjectsResponse{}, nil
}

func (c *fakeAuthorizationClient) ListUsers(context.Context, *authorizationv1.ListUsersRequest, ...grpc.CallOption) (*authorizationv1.ListUsersResponse, error) {
	return &authorizationv1.ListUsersResponse{}, nil
}

type fakeIdentityClient struct {
	types map[string]identityv1.IdentityType
}

func (c *fakeIdentityClient) RegisterIdentity(context.Context, *identityv1.RegisterIdentityRequest, ...grpc.CallOption) (*identityv1.RegisterIdentityResponse, error) {
	return &identityv1.RegisterIdentityResponse{}, nil
}

func (c *fakeIdentityClient) GetIdentityType(ctx context.Context, req *identityv1.GetIdentityTypeRequest, opts ...grpc.CallOption) (*identityv1.GetIdentityTypeResponse, error) {
	identityType, ok := c.types[req.GetIdentityId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "identity not found")
	}
	return &identityv1.GetIdentityTypeResponse{IdentityType: identityType}, nil
}

func (c *fakeIdentityClient) BatchGetIdentityTypes(context.Context, *identityv1.BatchGetIdentityTypesRequest, ...grpc.CallOption) (*identityv1.BatchGetIdentityTypesResponse, error) {
	return &identityv1.BatchGetIdentityTypesResponse{}, nil
}

func (c *fakeIdentityClient) SetNickname(context.Context, *identityv1.SetNicknameRequest, ...grpc.CallOption) (*identityv1.SetNicknameResponse, error) {
	return &identityv1.SetNicknameResponse{}, nil
}

func (c *fakeIdentityClient) RemoveNickname(context.Context, *identityv1.RemoveNicknameRequest, ...grpc.CallOption) (*identityv1.RemoveNicknameResponse, error) {
	return &identityv1.RemoveNicknameResponse{}, nil
}

func (c *fakeIdentityClient) ResolveNickname(context.Context, *identityv1.ResolveNicknameRequest, ...grpc.CallOption) (*identityv1.ResolveNicknameResponse, error) {
	return &identityv1.ResolveNicknameResponse{}, nil
}

func (c *fakeIdentityClient) BatchGetNicknames(context.Context, *identityv1.BatchGetNicknamesRequest, ...grpc.CallOption) (*identityv1.BatchGetNicknamesResponse, error) {
	return &identityv1.BatchGetNicknamesResponse{}, nil
}

type fakePublisher struct {
	addedErr   error
	removedErr error
	deletedErr error
	added      []*groupsv1.GroupMembershipAddedEvent
	removed    []*groupsv1.GroupMembershipRemovedEvent
	deleted    []*groupsv1.GroupDeletedEvent
}

func (p *fakePublisher) PublishMembershipAdded(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipAddedEvent) error {
	p.added = append(p.added, event)
	return p.addedErr
}

func (p *fakePublisher) PublishMembershipRemoved(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipRemovedEvent) error {
	p.removed = append(p.removed, event)
	return p.removedErr
}

func (p *fakePublisher) PublishGroupDeleted(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupDeletedEvent) error {
	p.deleted = append(p.deleted, event)
	return p.deletedErr
}

func TestGroupCRUDAndOpenFGATuples(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}
	publisher := &fakePublisher{}
	server := New(store, auth, identity, publisher)
	organizationID := uuid.New()

	created, err := server.CreateGroup(context.Background(), &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "Engineering",
		Description:    "Eng team",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Equal(t, "Engineering", created.GetGroup().GetName())
	require.Len(t, auth.writes, 1)
	require.Equal(t, "organization:"+organizationID.String(), auth.writes[0].GetWrites()[0].GetUser())
	require.Equal(t, "org", auth.writes[0].GetWrites()[0].GetRelation())

	updatedName := "Platform"
	updated, err := server.UpdateGroup(context.Background(), &groupsv1.UpdateGroupRequest{Id: created.GetGroup().GetMeta().GetId(), Name: &updatedName})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.GetGroup().GetName())

	got, err := server.GetGroup(context.Background(), &groupsv1.GetGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Equal(t, updatedName, got.GetGroup().GetName())

	listed, err := server.ListGroups(context.Background(), &groupsv1.ListGroupsRequest{OrganizationId: organizationID.String()})
	require.NoError(t, err)
	require.Len(t, listed.GetGroups(), 1)

	_, err = server.DeleteGroup(context.Background(), &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 2)
	require.Equal(t, "organization:"+organizationID.String(), auth.writes[1].GetDeletes()[0].GetUser())
	require.Len(t, publisher.deleted, 1)
}

func TestMembershipLifecycleAndBatchLookup(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	publisher := &fakePublisher{}
	server := New(store, auth, identity, publisher)
	organizationID := uuid.New()
	created, err := server.CreateGroup(context.Background(), &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "Engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := created.GetGroup().GetMeta().GetId()

	added, err := server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Equal(t, memberID.String(), added.GetMembership().GetMemberId())
	require.Len(t, auth.writes, 2)
	require.Equal(t, "identity:"+memberID.String(), auth.writes[1].GetWrites()[0].GetUser())
	require.Len(t, publisher.added, 1)

	duplicate, err := server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Equal(t, added.GetMembership().GetMeta().GetId(), duplicate.GetMembership().GetMeta().GetId())
	require.Len(t, auth.writes, 2)
	require.Len(t, publisher.added, 1)

	members, err := server.ListMembers(context.Background(), &groupsv1.ListMembersRequest{GroupId: groupID})
	require.NoError(t, err)
	require.Len(t, members.GetMemberships(), 1)

	memberGroups, err := server.ListMemberGroups(context.Background(), &groupsv1.ListMemberGroupsRequest{
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       memberID.String(),
		OrganizationId: organizationID.String(),
	})
	require.NoError(t, err)
	require.Len(t, memberGroups.GetGroups(), 1)

	batch, err := server.ListMemberGroupsBatch(context.Background(), &groupsv1.ListMemberGroupsBatchRequest{Members: []*groupsv1.ListMemberGroupsRequest{{
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       memberID.String(),
		OrganizationId: organizationID.String(),
	}}})
	require.NoError(t, err)
	require.Len(t, batch.GetEntries(), 1)
	require.Len(t, batch.GetEntries()[0].GetGroups(), 1)

	_, err = server.RemoveMember(context.Background(), &groupsv1.RemoveMemberRequest{GroupId: groupID, MemberId: memberID.String()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 3)
	require.Equal(t, "identity:"+memberID.String(), auth.writes[2].GetDeletes()[0].GetUser())
	require.Len(t, publisher.removed, 1)
}

func TestInvalidMembershipInputs(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	runnerID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{
		memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER,
		runnerID.String(): identityv1.IdentityType_IDENTITY_TYPE_RUNNER,
	}}
	server := New(store, auth, identity, &fakePublisher{})
	organizationID := uuid.New()
	created, err := server.CreateGroup(context.Background(), &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "Engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := created.GetGroup().GetMeta().GetId()

	_, err = server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_UNSPECIFIED,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   runnerID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_AGENT,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPublishFailureDoesNotRollbackMembership(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	publisher := &fakePublisher{addedErr: errors.New("publish failed")}
	server := New(store, auth, identity, publisher)
	organizationID := uuid.New()
	created, err := server.CreateGroup(context.Background(), &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "Engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)

	_, err = server.AddMember(context.Background(), &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Len(t, store.memberships, 1)
	require.Len(t, publisher.added, 1)
}
