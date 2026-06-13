package server

import (
	"context"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	"github.com/agynio/groups/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ListMemberGroups(ctx context.Context, req *groupsv1.ListMemberGroupsRequest) (*groupsv1.ListMemberGroupsResponse, error) {
	groups, nextToken, err := s.listMemberGroups(ctx, req)
	if err != nil {
		return nil, err
	}
	return &groupsv1.ListMemberGroupsResponse{Groups: groups, NextPageToken: nextToken}, nil
}

func (s *Server) ListMemberGroupsBatch(ctx context.Context, req *groupsv1.ListMemberGroupsBatchRequest) (*groupsv1.ListMemberGroupsBatchResponse, error) {
	members := req.GetMembers()
	entries := make([]*groupsv1.ListMemberGroupsBatchEntry, 0, len(members))
	for i, member := range members {
		if member.GetPageToken() != "" {
			return nil, status.Errorf(codes.InvalidArgument, "members[%d].page_token is not supported", i)
		}
		groups, _, err := s.listMemberGroups(ctx, member)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "members[%d]: %v", i, status.Convert(err).Message())
		}
		entries = append(entries, &groupsv1.ListMemberGroupsBatchEntry{
			MemberType: member.GetMemberType(),
			MemberId:   member.GetMemberId(),
			Groups:     groups,
		})
	}
	return &groupsv1.ListMemberGroupsBatchResponse{Entries: entries}, nil
}

func (s *Server) listMemberGroups(ctx context.Context, req *groupsv1.ListMemberGroupsRequest) ([]*groupsv1.Group, string, error) {
	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, "", status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	memberID, err := parseUUID(req.GetMemberId())
	if err != nil {
		return nil, "", status.Errorf(codes.InvalidArgument, "member_id: %v", err)
	}
	memberType, err := toStoreMemberType(req.GetMemberType())
	if err != nil {
		return nil, "", status.Errorf(codes.InvalidArgument, "member_type: %v", err)
	}
	if !isCaller(ctx, memberID) {
		if err := s.requireOrganizationMember(ctx, organizationID); err != nil {
			return nil, "", err
		}
	}
	cursor, err := decodeCursor(req.GetPageToken())
	if err != nil {
		return nil, "", toStatusError(err)
	}
	groups, nextCursor, err := s.store.ListMemberGroups(ctx, store.ListMemberGroupsFilter{
		MemberType:     memberType,
		MemberID:       memberID,
		OrganizationID: organizationID,
	}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, "", toStatusError(err)
	}
	protoGroups := make([]*groupsv1.Group, 0, len(groups))
	for _, group := range groups {
		protoGroups = append(protoGroups, toProtoGroup(group))
	}
	return protoGroups, encodeCursor(nextCursor), nil
}
