package server

import (
	"context"
	"log"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) CreateGroup(ctx context.Context, req *groupsv1.CreateGroupRequest) (*groupsv1.CreateGroupResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	caller, err := callerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireOrganizationOwner(ctx, organizationID); err != nil {
		return nil, err
	}
	if err := validateName(req.GetName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
	}
	source, err := toStoreGroupSource(req.GetSource())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "source: %v", err)
	}
	if err := validateExternalID(source, req.ExternalId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "external_id: %v", err)
	}

	group, err := s.store.CreateGroup(ctx, store.CreateGroupInput{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		Name:           req.GetName(),
		Description:    req.GetDescription(),
		Source:         source,
		ExternalID:     req.ExternalId,
	})
	if err != nil {
		return nil, toStatusError(err)
	}

	if err := s.writeGroupCreateTuples(ctx, group.Meta.ID, group.OrganizationID, caller.id); err != nil {
		_, _ = s.store.DeleteGroup(ctx, group.Meta.ID)
		return nil, status.Errorf(codes.Internal, "write group tuples: %v", err)
	}
	s.notifyGroupUpdated(ctx, group)

	return &groupsv1.CreateGroupResponse{Group: toProtoGroup(group)}, nil
}

func (s *Server) GetGroup(ctx context.Context, req *groupsv1.GetGroupRequest) (*groupsv1.GetGroupResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	group, err := s.store.GetGroup(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireOrganizationMember(ctx, group.OrganizationID); err != nil {
		return nil, err
	}
	return &groupsv1.GetGroupResponse{Group: toProtoGroup(group)}, nil
}

func (s *Server) ListGroups(ctx context.Context, req *groupsv1.ListGroupsRequest) (*groupsv1.ListGroupsResponse, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	if err := s.requireOrganizationMember(ctx, organizationID); err != nil {
		return nil, err
	}
	cursor, err := decodeCursor(req.GetPageToken())
	if err != nil {
		return nil, toStatusError(err)
	}
	filter := store.ListGroupsFilter{OrganizationID: organizationID}
	if req.Source != nil {
		source, err := toStoreGroupSource(req.GetSource())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "source: %v", err)
		}
		filter.Source = &source
	}

	groups, nextCursor, err := s.store.ListGroups(ctx, filter, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoGroups := make([]*groupsv1.Group, 0, len(groups))
	for _, group := range groups {
		protoGroups = append(protoGroups, toProtoGroup(group))
	}
	return &groupsv1.ListGroupsResponse{Groups: protoGroups, NextPageToken: encodeCursor(nextCursor)}, nil
}

func (s *Server) UpdateGroup(ctx context.Context, req *groupsv1.UpdateGroupRequest) (*groupsv1.UpdateGroupResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	group, err := s.store.GetGroup(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireOrganizationOwner(ctx, group.OrganizationID); err != nil {
		return nil, err
	}
	if req.Name != nil {
		if err := validateName(req.GetName()); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "name: %v", err)
		}
	}
	if req.Name == nil && req.Description == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}
	updated, err := s.store.UpdateGroup(ctx, store.UpdateGroupInput{ID: id, Name: req.Name, Description: req.Description})
	if err != nil {
		return nil, toStatusError(err)
	}
	s.notifyGroupUpdated(ctx, updated)
	return &groupsv1.UpdateGroupResponse{Group: toProtoGroup(updated)}, nil
}

func (s *Server) DeleteGroup(ctx context.Context, req *groupsv1.DeleteGroupRequest) (*groupsv1.DeleteGroupResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	group, err := s.store.GetGroup(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if err := s.requireOrganizationOwner(ctx, group.OrganizationID); err != nil {
		return nil, err
	}
	admins, err := s.listGroupAdmins(ctx, id)
	if err != nil {
		return nil, err
	}
	deleted := store.DeletedGroup{Group: group, Admins: admins}
	memberships, err := s.listAllMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	deleted.Memberships = memberships
	if err := s.deleteGroupTuples(ctx, deleted); err != nil {
		return nil, status.Errorf(codes.Internal, "delete group tuples: %v", err)
	}
	deleted, err = s.store.DeleteGroup(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	deleted.Admins = admins
	for _, membership := range deleted.Memberships {
		if err := s.publisher.PublishMembershipRemoved(ctx, uuid.New(), &groupsv1.GroupMembershipRemovedEvent{
			GroupId:    membership.GroupID.String(),
			MemberType: toProtoMemberType(membership.MemberType),
			MemberId:   membership.MemberID.String(),
		}); err != nil {
			log.Printf("publish membership removed failed during group delete (group=%s member=%s org=%s): %v", membership.GroupID, membership.MemberID, deleted.Group.OrganizationID, err)
		}
	}
	if err := s.publisher.PublishGroupDeleted(ctx, uuid.New(), &groupsv1.GroupDeletedEvent{
		GroupId:        deleted.Group.Meta.ID.String(),
		OrganizationId: deleted.Group.OrganizationID.String(),
	}); err != nil {
		log.Printf("publish group deleted failed (group=%s): %v", deleted.Group.Meta.ID, err)
	}
	s.notifyGroupUpdated(ctx, deleted.Group)
	return &groupsv1.DeleteGroupResponse{}, nil
}
