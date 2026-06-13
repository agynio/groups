package server

import (
	"context"
	"log"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) AddMember(ctx context.Context, req *groupsv1.AddMemberRequest) (*groupsv1.AddMemberResponse, error) {
	groupID, err := parseUUID(req.GetGroupId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "group_id: %v", err)
	}
	memberID, err := parseUUID(req.GetMemberId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "member_id: %v", err)
	}
	memberType, err := toStoreMemberType(req.GetMemberType())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "member_type: %v", err)
	}
	source, err := toStoreGroupSource(req.GetSource())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "source: %v", err)
	}
	group, err := s.store.GetGroup(ctx, groupID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireGroupEditor(ctx, group.Meta.ID); err != nil {
		return nil, err
	}
	if err := s.validateMemberIdentity(ctx, memberID, memberType, group.OrganizationID); err != nil {
		return nil, err
	}

	membership, created, err := s.store.AddMember(ctx, store.AddMemberInput{
		ID:         uuid.New(),
		GroupID:    group.Meta.ID,
		MemberType: memberType,
		MemberID:   memberID,
		Source:     source,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	if !created {
		return &groupsv1.AddMemberResponse{Membership: toProtoMembership(membership)}, nil
	}
	if err := s.writeMembershipTuple(ctx, membership); err != nil {
		_, _, _ = s.store.RemoveMember(ctx, membership.GroupID, membership.MemberID)
		return nil, status.Errorf(codes.Internal, "write membership tuple: %v", err)
	}
	if err := s.publisher.PublishMembershipAdded(ctx, uuid.New(), &groupsv1.GroupMembershipAddedEvent{
		GroupId:    membership.GroupID.String(),
		MemberType: toProtoMemberType(membership.MemberType),
		MemberId:   membership.MemberID.String(),
	}); err != nil {
		log.Printf("publish membership added failed (group=%s member=%s org=%s): %v", membership.GroupID, membership.MemberID, group.OrganizationID, err)
	}
	s.notifyMembershipUpdated(ctx, group.OrganizationID.String(), membership.GroupID.String(), membership.MemberID.String())
	return &groupsv1.AddMemberResponse{Membership: toProtoMembership(membership)}, nil
}

func (s *Server) RemoveMember(ctx context.Context, req *groupsv1.RemoveMemberRequest) (*groupsv1.RemoveMemberResponse, error) {
	groupID, err := parseUUID(req.GetGroupId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "group_id: %v", err)
	}
	memberID, err := parseUUID(req.GetMemberId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "member_id: %v", err)
	}
	group, err := s.store.GetGroup(ctx, groupID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireGroupEditor(ctx, groupID); err != nil {
		return nil, err
	}
	if err := s.checkOrganizationMember(ctx, memberID, group.OrganizationID); err != nil {
		return nil, err
	}
	removed, deleted, err := s.store.RemoveMember(ctx, groupID, memberID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !deleted {
		return &groupsv1.RemoveMemberResponse{}, nil
	}
	if err := s.deleteMembershipTuple(ctx, removed.Membership); err != nil {
		return nil, status.Errorf(codes.Internal, "delete membership tuple: %v", err)
	}
	if err := s.publisher.PublishMembershipRemoved(ctx, uuid.New(), &groupsv1.GroupMembershipRemovedEvent{
		GroupId:    removed.Membership.GroupID.String(),
		MemberType: toProtoMemberType(removed.Membership.MemberType),
		MemberId:   removed.Membership.MemberID.String(),
	}); err != nil {
		log.Printf("publish membership removed failed (group=%s member=%s org=%s): %v", removed.Membership.GroupID, removed.Membership.MemberID, removed.OrganizationID, err)
	}
	s.notifyMembershipUpdated(ctx, removed.OrganizationID.String(), removed.Membership.GroupID.String(), removed.Membership.MemberID.String())
	return &groupsv1.RemoveMemberResponse{}, nil
}

func (s *Server) ListMembers(ctx context.Context, req *groupsv1.ListMembersRequest) (*groupsv1.ListMembersResponse, error) {
	groupID, err := parseUUID(req.GetGroupId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "group_id: %v", err)
	}
	if _, err := s.store.GetGroup(ctx, groupID); err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireGroupViewer(ctx, groupID); err != nil {
		return nil, err
	}
	cursor, err := decodeCursor(req.GetPageToken())
	if err != nil {
		return nil, toStatusError(err)
	}
	filter := store.ListMembersFilter{GroupID: groupID}
	if req.MemberType != nil {
		memberType, err := toStoreMemberType(req.GetMemberType())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "member_type: %v", err)
		}
		filter.MemberType = &memberType
	}
	memberships, nextCursor, err := s.store.ListMembers(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoMemberships := make([]*groupsv1.GroupMembership, 0, len(memberships))
	for _, membership := range memberships {
		protoMemberships = append(protoMemberships, toProtoMembership(membership))
	}
	return &groupsv1.ListMembersResponse{Memberships: protoMemberships, NextPageToken: encodeCursor(nextCursor)}, nil
}

func (s *Server) validateMemberIdentity(ctx context.Context, memberID uuid.UUID, memberType store.GroupMemberType, organizationID uuid.UUID) error {
	response, err := s.identityClient.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: memberID.String()})
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "member_id identity lookup: %v", err)
	}
	if response.GetIdentityType() == identityv1.IdentityType_IDENTITY_TYPE_RUNNER {
		return status.Error(codes.InvalidArgument, "runner identities cannot be group members")
	}
	expected := expectedIdentityType(memberType)
	if response.GetIdentityType() != expected {
		return status.Errorf(codes.InvalidArgument, "member_type does not match identity type %s", response.GetIdentityType().String())
	}
	if err := s.checkOrganizationMember(ctx, memberID, organizationID); err != nil {
		return err
	}
	return nil
}
