package server

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	notificationsv1 "github.com/agynio/groups/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeStore struct {
	groups      map[uuid.UUID]store.Group
	memberships map[uuid.UUID]store.GroupMembership
	deleted     []uuid.UUID
	now         time.Time
	deleteErr   error
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

func (s *fakeStore) ListAllGroups(ctx context.Context) ([]store.Group, error) {
	groups := make([]store.Group, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}
	return groups, nil
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
	if s.deleteErr != nil {
		return store.DeletedGroup{}, s.deleteErr
	}
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

func (s *fakeStore) GetMembershipByGroupMember(ctx context.Context, groupID uuid.UUID, memberID uuid.UUID) (store.GroupMembership, error) {
	for _, membership := range s.memberships {
		if membership.GroupID == groupID && membership.MemberID == memberID {
			return membership, nil
		}
	}
	return store.GroupMembership{}, store.NotFound("membership")
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
	return pageMemberships(memberships, pageSize, cursor)
}

func (s *fakeStore) ListAllMemberships(ctx context.Context) ([]store.GroupMembership, error) {
	memberships := make([]store.GroupMembership, 0, len(s.memberships))
	for _, membership := range s.memberships {
		memberships = append(memberships, membership)
	}
	return memberships, nil
}

func pageMemberships(memberships []store.GroupMembership, pageSize int32, cursor *store.PageCursor) ([]store.GroupMembership, *store.PageCursor, error) {
	sort.Slice(memberships, func(i int, j int) bool {
		return memberships[i].Meta.ID.String() < memberships[j].Meta.ID.String()
	})
	limit := store.NormalizePageSize(pageSize)
	start := 0
	if cursor != nil {
		start = len(memberships)
		for i, membership := range memberships {
			if membership.Meta.ID.String() > cursor.AfterID.String() {
				start = i
				break
			}
		}
	}
	end := start + int(limit)
	if end >= len(memberships) {
		return memberships[start:], nil, nil
	}
	return memberships[start:end], &store.PageCursor{AfterID: memberships[end-1].Meta.ID}, nil
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
	checks      map[string]bool
	checkedKeys []*authorizationv1.TupleKey
	writes      []*authorizationv1.WriteRequest
	readTuples  []*authorizationv1.Tuple
	readPages   []*authorizationv1.ReadResponse
	checkErr    error
	readErr     error
	writeErr    error
}

func (c *fakeAuthorizationClient) Check(ctx context.Context, req *authorizationv1.CheckRequest, opts ...grpc.CallOption) (*authorizationv1.CheckResponse, error) {
	if c.checkErr != nil {
		return nil, c.checkErr
	}
	c.checkedKeys = append(c.checkedKeys, req.GetTupleKey())
	key := tupleKeyString(req.GetTupleKey())
	allowed, ok := c.checks[key]
	if !ok {
		allowed = true
	}
	return &authorizationv1.CheckResponse{Allowed: allowed}, nil
}

func (c *fakeAuthorizationClient) BatchCheck(context.Context, *authorizationv1.BatchCheckRequest, ...grpc.CallOption) (*authorizationv1.BatchCheckResponse, error) {
	return &authorizationv1.BatchCheckResponse{}, nil
}

func (c *fakeAuthorizationClient) Write(ctx context.Context, req *authorizationv1.WriteRequest, opts ...grpc.CallOption) (*authorizationv1.WriteResponse, error) {
	if c.writeErr != nil {
		return nil, c.writeErr
	}
	c.writes = append(c.writes, req)
	return &authorizationv1.WriteResponse{}, nil
}

func (c *fakeAuthorizationClient) Read(_ context.Context, req *authorizationv1.ReadRequest, _ ...grpc.CallOption) (*authorizationv1.ReadResponse, error) {
	if c.readErr != nil {
		return nil, c.readErr
	}
	if len(c.readPages) > 0 {
		response := c.readPages[0]
		c.readPages = c.readPages[1:]
		return response, nil
	}
	tuples := append([]*authorizationv1.Tuple{}, c.readTuples...)
	for _, write := range c.writes {
		for _, tupleKey := range write.GetWrites() {
			if tupleMatches(req.GetTupleKey(), tupleKey) {
				tuples = append(tuples, &authorizationv1.Tuple{Key: tupleKey})
			}
		}
		for _, tupleKey := range write.GetDeletes() {
			kept := tuples[:0]
			for _, tuple := range tuples {
				if tupleKeyString(tuple.GetKey()) != tupleKeyString(tupleKey) {
					kept = append(kept, tuple)
				}
			}
			tuples = kept
		}
	}
	return &authorizationv1.ReadResponse{Tuples: tuples}, nil
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
	subjects   []string
}

func (p *fakePublisher) PublishMembershipAdded(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipAddedEvent) error {
	p.added = append(p.added, event)
	p.subjects = append(p.subjects, "membership.added")
	return p.addedErr
}

func (p *fakePublisher) PublishMembershipRemoved(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipRemovedEvent) error {
	p.removed = append(p.removed, event)
	p.subjects = append(p.subjects, "membership.removed")
	return p.removedErr
}

func (p *fakePublisher) PublishGroupDeleted(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupDeletedEvent) error {
	p.deleted = append(p.deleted, event)
	p.subjects = append(p.subjects, "group.deleted")
	return p.deletedErr
}

type fakeNotificationsClient struct {
	published []*notificationsv1.PublishRequest
	err       error
}

func (c *fakeNotificationsClient) Publish(ctx context.Context, req *notificationsv1.PublishRequest, opts ...grpc.CallOption) (*notificationsv1.PublishResponse, error) {
	c.published = append(c.published, req)
	return &notificationsv1.PublishResponse{}, c.err
}

func (c *fakeNotificationsClient) Subscribe(ctx context.Context, req *notificationsv1.SubscribeRequest, opts ...grpc.CallOption) (notificationsv1.NotificationsService_SubscribeClient, error) {
	return nil, errors.New("unexpected subscribe")
}

func TestGroupCRUDAndOpenFGATuples(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}
	publisher := &fakePublisher{}
	notifications := &fakeNotificationsClient{}
	server := NewWithNotifications(store, auth, identity, notifications, publisher)
	callerID := uuid.New()
	organizationID := uuid.New()
	ctx := contextWithIdentity(callerID)

	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Description:    "eng team",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Equal(t, "engineering", created.GetGroup().GetName())
	require.Len(t, auth.writes, 1)
	require.Len(t, auth.writes[0].GetWrites(), 2)
	require.Equal(t, "group:"+created.GetGroup().GetMeta().GetId(), auth.writes[0].GetWrites()[0].GetUser())
	require.Equal(t, "organization:"+organizationID.String(), auth.writes[0].GetWrites()[0].GetObject())
	require.Equal(t, "org", auth.writes[0].GetWrites()[0].GetRelation())
	require.Equal(t, identityObject(callerID), auth.writes[0].GetWrites()[1].GetUser())
	require.Equal(t, groupAdminRelation, auth.writes[0].GetWrites()[1].GetRelation())
	require.Equal(t, "group:"+created.GetGroup().GetMeta().GetId(), auth.writes[0].GetWrites()[1].GetObject())
	require.Len(t, notifications.published, 1)
	require.Equal(t, notificationEventGroupUpdated, notifications.published[0].GetEvent())

	updatedName := "platform"
	updated, err := server.UpdateGroup(ctx, &groupsv1.UpdateGroupRequest{Id: created.GetGroup().GetMeta().GetId(), Name: &updatedName})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.GetGroup().GetName())

	got, err := server.GetGroup(ctx, &groupsv1.GetGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Equal(t, updatedName, got.GetGroup().GetName())

	listed, err := server.ListGroups(ctx, &groupsv1.ListGroupsRequest{OrganizationId: organizationID.String()})
	require.NoError(t, err)
	require.Len(t, listed.GetGroups(), 1)

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 2)
	require.Len(t, auth.writes[1].GetDeletes(), 2)
	require.Equal(t, "group:"+created.GetGroup().GetMeta().GetId(), auth.writes[1].GetDeletes()[0].GetUser())
	require.Equal(t, "organization:"+organizationID.String(), auth.writes[1].GetDeletes()[0].GetObject())
	require.Equal(t, identityObject(callerID), auth.writes[1].GetDeletes()[1].GetUser())
	require.Equal(t, groupAdminRelation, auth.writes[1].GetDeletes()[1].GetRelation())
	require.Len(t, publisher.deleted, 1)
}

func TestMembershipLifecycleAndBatchLookup(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	publisher := &fakePublisher{}
	notifications := &fakeNotificationsClient{}
	server := NewWithNotifications(store, auth, identity, notifications, publisher)
	callerID := uuid.New()
	organizationID := uuid.New()
	ctx := contextWithIdentity(callerID)
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := created.GetGroup().GetMeta().GetId()

	added, err := server.AddMember(ctx, &groupsv1.AddMemberRequest{
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

	duplicate, err := server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Equal(t, added.GetMembership().GetMeta().GetId(), duplicate.GetMembership().GetMeta().GetId())
	require.Len(t, auth.writes, 2)
	require.Len(t, publisher.added, 1)

	members, err := server.ListMembers(ctx, &groupsv1.ListMembersRequest{GroupId: groupID})
	require.NoError(t, err)
	require.Len(t, members.GetMemberships(), 1)

	memberGroups, err := server.ListMemberGroups(contextWithIdentity(memberID), &groupsv1.ListMemberGroupsRequest{
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       memberID.String(),
		OrganizationId: organizationID.String(),
	})
	require.NoError(t, err)
	require.Len(t, memberGroups.GetGroups(), 1)

	batch, err := server.ListMemberGroupsBatch(contextWithIdentity(memberID), &groupsv1.ListMemberGroupsBatchRequest{Members: []*groupsv1.ListMemberGroupsRequest{{
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       memberID.String(),
		OrganizationId: organizationID.String(),
	}}})
	require.NoError(t, err)
	require.Len(t, batch.GetEntries(), 1)
	require.Len(t, batch.GetEntries()[0].GetGroups(), 1)

	_, err = server.RemoveMember(ctx, &groupsv1.RemoveMemberRequest{GroupId: groupID, MemberId: memberID.String()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 3)
	require.Equal(t, "identity:"+memberID.String(), auth.writes[2].GetDeletes()[0].GetUser())
	require.Len(t, publisher.removed, 1)
	require.Contains(t, notifications.published[len(notifications.published)-1].GetRooms(), "organization:"+organizationID.String())
}

func TestDeleteGroupDeletesAdminTuples(t *testing.T) {
	store := newFakeStore()
	adminID := uuid.New()
	auth := &fakeAuthorizationClient{}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx := contextWithIdentity(adminID)
	organizationID := uuid.New()
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 2)
	require.Len(t, auth.writes[1].GetDeletes(), 2)
	require.Equal(t, identityObject(adminID), auth.writes[1].GetDeletes()[1].GetUser())
	require.Equal(t, groupAdminRelation, auth.writes[1].GetDeletes()[1].GetRelation())
	require.Equal(t, groupObject(uuid.MustParse(created.GetGroup().GetMeta().GetId())), auth.writes[1].GetDeletes()[1].GetObject())
}

func TestDeleteGroupCleansTuplesBeforeStoreDelete(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	store.deleteErr = errors.New("store delete failed")

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Len(t, auth.writes, 2)
	require.NotEmpty(t, auth.writes[1].GetDeletes())
	require.Contains(t, store.groups, uuid.MustParse(created.GetGroup().GetMeta().GetId()))
}

func TestDeleteGroupKeepsStoreStateWhenTupleCleanupFails(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	auth.writeErr = errors.New("tuple cleanup failed")

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Contains(t, store.groups, uuid.MustParse(created.GetGroup().GetMeta().GetId()))
}

func TestDeleteGroupCleansTuplesForMoreThanOneMembershipPage(t *testing.T) {
	groupStore := newFakeStore()
	auth := &fakeAuthorizationClient{}
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := uuid.MustParse(created.GetGroup().GetMeta().GetId())
	const membershipCount = int(store.DefaultListPageSize) + 1
	for i := 0; i < membershipCount; i++ {
		memberID := uuid.New()
		groupStore.memberships[uuid.New()] = store.GroupMembership{
			Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
			GroupID:    groupID,
			MemberType: store.GroupMemberTypeUser,
			MemberID:   memberID,
			Source:     store.GroupSourcePlatform,
		}
	}

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: groupID.String()})
	require.NoError(t, err)
	require.Len(t, auth.writes, 2)
	require.Len(t, auth.writes[1].GetDeletes(), membershipCount+2)
}

func TestCreateGroupRollbackOnTupleWriteFailure(t *testing.T) {
	store := newFakeStore()
	server := New(store, &fakeAuthorizationClient{writeErr: errors.New("write failed")}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})

	_, err := server.CreateGroup(contextWithIdentity(uuid.New()), &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Empty(t, store.groups)
}

func TestAddMemberRollbackOnTupleWriteFailure(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	auth.writeErr = errors.New("write failed")

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Empty(t, store.memberships)
}

func TestAuthorizationDenied(t *testing.T) {
	store := newFakeStore()
	organizationID := uuid.New()
	callerID := uuid.New()
	auth := &fakeAuthorizationClient{checks: map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(callerID), Relation: organizationOwnerRelation, Object: organizationObject(organizationID)}): false,
	}}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})

	_, err := server.CreateGroup(contextWithIdentity(callerID), &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAuthorizationChecksForGroupAPIs(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}
	server := New(store, auth, identity, &fakePublisher{})
	callerID := uuid.New()
	organizationID := uuid.New()
	ctx := contextWithIdentity(callerID)
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := uuid.MustParse(created.GetGroup().GetMeta().GetId())

	_, err = server.GetGroup(ctx, &groupsv1.GetGroupRequest{Id: groupID.String()})
	require.NoError(t, err)
	_, err = server.ListGroups(ctx, &groupsv1.ListGroupsRequest{OrganizationId: organizationID.String()})
	require.NoError(t, err)
	updatedName := "engineering-team"
	updated, err := server.UpdateGroup(ctx, &groupsv1.UpdateGroupRequest{Id: groupID.String(), Name: &updatedName})
	require.NoError(t, err)
	_, err = server.ListMembers(ctx, &groupsv1.ListMembersRequest{GroupId: groupID.String()})
	require.NoError(t, err)
	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: groupID.String()})
	require.NoError(t, err)
	memberID := uuid.New()
	server.identityClient.(*fakeIdentityClient).types[memberID.String()] = identityv1.IdentityType_IDENTITY_TYPE_USER
	created, err = server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering-next",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID = uuid.MustParse(created.GetGroup().GetMeta().GetId())
	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    groupID.String(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	_, err = server.ListMemberGroups(ctx, &groupsv1.ListMemberGroupsRequest{
		OrganizationId: organizationID.String(),
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       uuid.New().String(),
	})
	require.NoError(t, err)

	requireCheck(t, auth, identityObject(callerID), organizationOwnerRelation, organizationObject(organizationID))
	requireCheck(t, auth, identityObject(callerID), organizationMemberRelation, organizationObject(organizationID))
	requireCheck(t, auth, identityObject(callerID), groupCanViewRelation, groupObject(uuid.MustParse(updated.GetGroup().GetMeta().GetId())))
	requireCheck(t, auth, identityObject(callerID), groupCanEditRelation, groupObject(groupID))
}

func TestUpdateAndDeleteUseOrganizationOwnerAuthorization(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	adminID := uuid.New()
	organizationID := uuid.New()
	ctx := contextWithIdentity(adminID)
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := uuid.MustParse(created.GetGroup().GetMeta().GetId())
	auth.checks = map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(adminID), Relation: organizationOwnerRelation, Object: organizationObject(organizationID)}): true,
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(adminID), Relation: groupCanEditRelation, Object: groupObject(groupID)}):                    false,
	}
	updatedName := "engineering-admin"

	_, err = server.UpdateGroup(ctx, &groupsv1.UpdateGroupRequest{Id: groupID.String(), Name: &updatedName})
	require.NoError(t, err)
	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: groupID.String()})
	require.NoError(t, err)
	requireCheck(t, auth, identityObject(adminID), organizationOwnerRelation, organizationObject(organizationID))
}

func TestListMemberGroupsOtherIdentityDenied(t *testing.T) {
	store := newFakeStore()
	organizationID := uuid.New()
	callerID := uuid.New()
	auth := &fakeAuthorizationClient{checks: map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(callerID), Relation: organizationMemberRelation, Object: organizationObject(organizationID)}): false,
	}}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})

	_, err := server.ListMemberGroups(contextWithIdentity(callerID), &groupsv1.ListMemberGroupsRequest{
		OrganizationId: organizationID.String(),
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       uuid.New().String(),
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestListMemberGroupsBatchEnforcesEachEntryAuthorization(t *testing.T) {
	store := newFakeStore()
	organizationID := uuid.New()
	callerID := uuid.New()
	otherMemberID := uuid.New()
	auth := &fakeAuthorizationClient{checks: map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(callerID), Relation: organizationMemberRelation, Object: organizationObject(organizationID)}): false,
	}}
	server := New(store, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})

	_, err := server.ListMemberGroupsBatch(contextWithIdentity(callerID), &groupsv1.ListMemberGroupsBatchRequest{Members: []*groupsv1.ListMemberGroupsRequest{{
		OrganizationId: organizationID.String(),
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       callerID.String(),
	}, {
		OrganizationId: organizationID.String(),
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       otherMemberID.String(),
	}}})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "members[1]")
}

func TestListMemberGroupsBatchRequiresAuthentication(t *testing.T) {
	server := New(newFakeStore(), &fakeAuthorizationClient{}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})

	_, err := server.ListMemberGroupsBatch(context.Background(), &groupsv1.ListMemberGroupsBatchRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestListMemberGroupsSelfRequiresAuthentication(t *testing.T) {
	server := New(newFakeStore(), &fakeAuthorizationClient{}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	memberID := uuid.New()

	_, err := server.ListMemberGroups(context.Background(), &groupsv1.ListMemberGroupsRequest{
		OrganizationId: uuid.New().String(),
		MemberType:     groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:       memberID.String(),
	})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCreateGroupSourceValidation(t *testing.T) {
	server := New(newFakeStore(), &fakeAuthorizationClient{}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	externalID := "idp-group-1"

	_, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
		ExternalId:     &externalID,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "sales",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_SCIM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestSameOrgGuardRejectsCrossOrgMember(t *testing.T) {
	store := newFakeStore()
	organizationID := uuid.New()
	callerID := uuid.New()
	memberID := uuid.New()
	auth := &fakeAuthorizationClient{checks: map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(memberID), Relation: organizationMemberRelation, Object: organizationObject(organizationID)}): false,
	}}
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	server := New(store, auth, identity, &fakePublisher{})
	ctx := contextWithIdentity(callerID)
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRemoveMemberRejectsCrossOrgMember(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	memberID := uuid.New()
	auth := &fakeAuthorizationClient{checks: map[string]bool{
		tupleKeyString(&authorizationv1.TupleKey{User: identityObject(memberID), Relation: organizationMemberRelation, Object: organizationObject(organizationID)}): false,
	}}
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	server := New(groupStore, auth, identity, &fakePublisher{})
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := uuid.MustParse(created.GetGroup().GetMeta().GetId())
	groupStore.memberships[uuid.New()] = store.GroupMembership{
		Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		GroupID:    groupID,
		MemberType: store.GroupMemberTypeUser,
		MemberID:   memberID,
		Source:     store.GroupSourcePlatform,
	}

	_, err = server.RemoveMember(ctx, &groupsv1.RemoveMemberRequest{GroupId: groupID.String(), MemberId: memberID.String()})
	require.NoError(t, err)
	require.Empty(t, groupStore.memberships)
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
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	groupID := created.GetGroup().GetMeta().GetId()

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_UNSPECIFIED,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   runnerID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    groupID,
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_AGENT,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestDeleteGroupPublishesMembershipRemovedEvents(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	secondMemberID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{
		memberID.String():       identityv1.IdentityType_IDENTITY_TYPE_USER,
		secondMemberID.String(): identityv1.IdentityType_IDENTITY_TYPE_APP,
	}}
	publisher := &fakePublisher{}
	server := New(store, auth, identity, publisher)
	organizationID := uuid.New()
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_APP,
		MemberId:   secondMemberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	publisher.subjects = nil

	_, err = server.DeleteGroup(ctx, &groupsv1.DeleteGroupRequest{Id: created.GetGroup().GetMeta().GetId()})
	require.NoError(t, err)
	require.Len(t, publisher.removed, 2)
	require.Len(t, publisher.deleted, 1)
	require.Equal(t, []string{"membership.removed", "membership.removed", "group.deleted"}, publisher.subjects)
}

func TestPublishFailureDoesNotRollbackMembership(t *testing.T) {
	store := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	identity := &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}
	publisher := &fakePublisher{addedErr: errors.New("publish failed")}
	server := New(store, auth, identity, publisher)
	organizationID := uuid.New()
	ctx := contextWithIdentity(uuid.New())
	created, err := server.CreateGroup(ctx, &groupsv1.CreateGroupRequest{
		OrganizationId: organizationID.String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)

	_, err = server.AddMember(ctx, &groupsv1.AddMemberRequest{
		GroupId:    created.GetGroup().GetMeta().GetId(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   memberID.String(),
		Source:     groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Len(t, store.memberships, 1)
	require.Len(t, publisher.added, 1)
}

func TestNotificationFailureDoesNotRollbackGroupCreate(t *testing.T) {
	store := newFakeStore()
	notifications := &fakeNotificationsClient{err: errors.New("notify failed")}
	server := NewWithNotifications(store, &fakeAuthorizationClient{}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, notifications, &fakePublisher{})

	created, err := server.CreateGroup(contextWithIdentity(uuid.New()), &groupsv1.CreateGroupRequest{
		OrganizationId: uuid.New().String(),
		Name:           "engineering",
		Source:         groupsv1.GroupSource_GROUP_SOURCE_PLATFORM,
	})
	require.NoError(t, err)
	require.Contains(t, store.groups, uuid.MustParse(created.GetGroup().GetMeta().GetId()))
	require.Len(t, notifications.published, 1)
}

func TestReconcileRepairsMissingGroupAndMembershipTuples(t *testing.T) {
	groupStore := newFakeStore()
	auth := &fakeAuthorizationClient{}
	memberID := uuid.New()
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}, &fakePublisher{})
	organizationID := uuid.New()
	groupID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}
	groupStore.memberships[uuid.New()] = store.GroupMembership{
		Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		GroupID:    groupID,
		MemberType: store.GroupMemberTypeUser,
		MemberID:   memberID,
		Source:     store.GroupSourcePlatform,
	}

	report, err := server.Reconcile(context.Background())
	require.NoError(t, err)
	require.Equal(t, ReconciliationReport{GroupsScanned: 1, MembershipsScanned: 1, TuplesWritten: 2}, report)
	require.Len(t, auth.writes, 2)
	require.Equal(t, groupOrgTuple(groupID, organizationID), auth.writes[0].GetWrites()[0])
	require.Equal(t, membershipTuple(firstMembership(groupStore)), auth.writes[1].GetWrites()[0])
}

func TestReconcileDeletesStaleGroupTuples(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	groupID := uuid.New()
	memberID := uuid.New()
	staleGroupID := uuid.New()
	staleMemberID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}
	membership := store.GroupMembership{
		Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		GroupID:    groupID,
		MemberType: store.GroupMemberTypeUser,
		MemberID:   memberID,
		Source:     store.GroupSourcePlatform,
	}
	groupStore.memberships[membership.Meta.ID] = membership
	auth := &fakeAuthorizationClient{readTuples: []*authorizationv1.Tuple{
		{Key: groupOrgTuple(groupID, organizationID)},
		{Key: membershipTuple(membership)},
		{Key: groupOrgTuple(staleGroupID, organizationID)},
		{Key: &authorizationv1.TupleKey{User: identityObject(staleMemberID), Relation: memberRelation, Object: groupObject(groupID)}},
	}}
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}, &fakePublisher{})

	report, err := server.Reconcile(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, report.TuplesWritten)
	require.Equal(t, 2, report.TuplesDeleted)
	require.Len(t, auth.writes, 1)
	require.ElementsMatch(t, []*authorizationv1.TupleKey{
		groupOrgTuple(staleGroupID, organizationID),
		{User: identityObject(staleMemberID), Relation: memberRelation, Object: groupObject(groupID)},
	}, auth.writes[0].GetDeletes())
}

func TestReconcileRemovesOrphanedMembership(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	groupID := uuid.New()
	memberID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}
	membership := store.GroupMembership{
		Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		GroupID:    groupID,
		MemberType: store.GroupMemberTypeUser,
		MemberID:   memberID,
		Source:     store.GroupSourcePlatform,
	}
	groupStore.memberships[membership.Meta.ID] = membership
	auth := &fakeAuthorizationClient{readTuples: []*authorizationv1.Tuple{{Key: groupOrgTuple(groupID, organizationID)}, {Key: membershipTuple(membership)}}}
	publisher := &fakePublisher{}
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, publisher)

	report, err := server.Reconcile(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, report.OrphanedMembershipsRemoved)
	require.Equal(t, 1, report.TuplesDeleted)
	require.Empty(t, groupStore.memberships)
	require.Len(t, publisher.removed, 1)
	require.Len(t, auth.writes, 1)
	require.Len(t, auth.writes[0].GetDeletes(), 1)
	require.Equal(t, membershipTuple(membership), auth.writes[0].GetDeletes()[0])
}

func TestReconcileIsIdempotentWhenTuplesMatch(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	groupID := uuid.New()
	memberID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}
	membership := store.GroupMembership{
		Meta:       store.EntityMeta{ID: uuid.New(), CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		GroupID:    groupID,
		MemberType: store.GroupMemberTypeUser,
		MemberID:   memberID,
		Source:     store.GroupSourcePlatform,
	}
	groupStore.memberships[membership.Meta.ID] = membership
	auth := &fakeAuthorizationClient{readTuples: []*authorizationv1.Tuple{{Key: groupOrgTuple(groupID, organizationID)}, {Key: membershipTuple(membership)}}}
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{memberID.String(): identityv1.IdentityType_IDENTITY_TYPE_USER}}, &fakePublisher{})

	report, err := server.Reconcile(context.Background())
	require.NoError(t, err)
	require.Equal(t, ReconciliationReport{GroupsScanned: 1, MembershipsScanned: 1}, report)
	require.Empty(t, auth.writes)
}

func TestReconcileReturnsReadAndWriteErrors(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	groupID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}

	server := New(groupStore, &fakeAuthorizationClient{readErr: errors.New("read failed")}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	_, err := server.Reconcile(context.Background())
	require.Equal(t, codes.Internal, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "read failed")

	server = New(groupStore, &fakeAuthorizationClient{writeErr: errors.New("write failed")}, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	_, err = server.Reconcile(context.Background())
	require.Equal(t, codes.Internal, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "write failed")
}

func contextWithIdentity(identityID uuid.UUID) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(identityIDMetadataKey, identityID.String()))
}

func firstMembership(groupStore *fakeStore) store.GroupMembership {
	for _, membership := range groupStore.memberships {
		return membership
	}
	panic("expected membership")
}

func requireCheck(t *testing.T, auth *fakeAuthorizationClient, user string, relation string, object string) {
	t.Helper()
	want := &authorizationv1.TupleKey{User: user, Relation: relation, Object: object}
	for _, checked := range auth.checkedKeys {
		if tupleKeyString(checked) == tupleKeyString(want) {
			return
		}
	}
	require.Failf(t, "missing authorization check", "%s", tupleKeyString(want))
}

func tupleMatches(filter *authorizationv1.TupleKey, tuple *authorizationv1.TupleKey) bool {
	if filter.GetUser() != "" && filter.GetUser() != tuple.GetUser() {
		return false
	}
	if filter.GetRelation() != "" && filter.GetRelation() != tuple.GetRelation() {
		return false
	}
	if filter.GetObject() != "" && filter.GetObject() != tuple.GetObject() {
		return false
	}
	return true
}
