package server

import (
	"context"
	"fmt"
	"strings"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	identityIDMetadataKey       = "x-identity-id"
	identityTypeMetadataKey     = "x-identity-type"
	legacyIdentityIDMetadataKey = "identity_id"
	groupAdminRelation          = "admin"
	organizationOwnerRelation   = "owner"
	organizationMemberRelation  = "member"
	groupCanEditRelation        = "can_edit"
	groupCanViewRelation        = "can_view"
	platformIdentityType        = "platform"
	clusterAdminRelation        = "admin"
	clusterObject               = "cluster:global"
)

type callerIdentity struct {
	id uuid.UUID
}

func callerFromContext(ctx context.Context) (callerIdentity, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return callerIdentity{}, status.Error(codes.Unauthenticated, "missing identity metadata")
	}
	identityID := firstMetadataValue(md, identityIDMetadataKey)
	if identityID == "" {
		identityID = firstMetadataValue(md, legacyIdentityIDMetadataKey)
	}
	if strings.TrimSpace(identityID) == "" {
		return callerIdentity{}, status.Error(codes.Unauthenticated, "missing identity id")
	}
	id, err := parseUUID(identityID)
	if err != nil {
		return callerIdentity{}, status.Errorf(codes.Unauthenticated, "identity id: %v", err)
	}
	return callerIdentity{id: id}, nil
}

func firstMetadataValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *Server) requireOrganizationOwner(ctx context.Context, organizationID uuid.UUID) error {
	return s.requireAllowed(ctx, organizationOwnerRelation, organizationObject(organizationID))
}

// requireOrganizationMember gates the read paths. A platform service reading
// them is admitted as itself: the Orchestrator resolves an agent's groups to
// build its workload, and it is not a member of the organization that agent
// belongs to. It used to send that agent's own id as the caller to get past
// this, which is the one thing a platform service must not do.
//
// Reads only. The write paths are owner-gated and stay that way -- the platform
// configures workloads from groups, it does not decide who is in one.
func (s *Server) requireOrganizationMember(ctx context.Context, organizationID uuid.UUID) error {
	allowed, err := s.platformAllowed(ctx)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	return s.requireAllowed(ctx, organizationMemberRelation, organizationObject(organizationID))
}

// platformAllowed reports whether the caller is the platform itself. The
// identity type arrives as metadata and settles nothing on its own; what admits
// the caller is the cluster admin tuple behind it, which Identity writes for one
// configured id and nothing can request.
func (s *Server) platformAllowed(ctx context.Context) (bool, error) {
	if identityTypeFromContext(ctx) != platformIdentityType {
		return false, nil
	}
	caller, err := callerFromContext(ctx)
	if err != nil {
		return false, err
	}
	response, err := s.authorizationClient.Check(ctx, &authorizationv1.CheckRequest{TupleKey: &authorizationv1.TupleKey{
		User:     identityObject(caller.id),
		Relation: clusterAdminRelation,
		Object:   clusterObject,
	}})
	if err != nil {
		return false, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	return response.GetAllowed(), nil
}

func identityTypeFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(firstMetadataValue(md, identityTypeMetadataKey))
}

func (s *Server) requireGroupEditor(ctx context.Context, groupID uuid.UUID) error {
	return s.requireAllowed(ctx, groupCanEditRelation, groupObject(groupID))
}

func (s *Server) requireGroupViewer(ctx context.Context, groupID uuid.UUID) error {
	return s.requireAllowed(ctx, groupCanViewRelation, groupObject(groupID))
}

func (s *Server) requireAllowed(ctx context.Context, relation string, object string) error {
	caller, err := callerFromContext(ctx)
	if err != nil {
		return err
	}
	return s.checkAllowed(ctx, caller.id, relation, object, codes.PermissionDenied, "permission denied")
}

func (s *Server) checkAllowed(ctx context.Context, identityID uuid.UUID, relation string, object string, deniedCode codes.Code, deniedMessage string) error {
	response, err := s.authorizationClient.Check(ctx, &authorizationv1.CheckRequest{TupleKey: &authorizationv1.TupleKey{
		User:     identityObject(identityID),
		Relation: relation,
		Object:   object,
	}})
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !response.GetAllowed() {
		return status.Error(deniedCode, deniedMessage)
	}
	return nil
}

func (s *Server) checkOrganizationMember(ctx context.Context, identityID uuid.UUID, organizationID uuid.UUID) error {
	return s.checkAllowed(ctx, identityID, organizationMemberRelation, organizationObject(organizationID), codes.InvalidArgument, "member does not belong to the group organization")
}

func (s *Server) listGroupAdmins(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	admins := []uuid.UUID{}
	pageToken := ""
	for {
		response, err := s.authorizationClient.Read(ctx, &authorizationv1.ReadRequest{
			TupleKey:  &authorizationv1.TupleKey{Relation: groupAdminRelation, Object: groupObject(groupID)},
			PageSize:  100,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "list group admins: %v", err)
		}
		for _, tuple := range response.GetTuples() {
			adminID, err := parseIdentityObject(tuple.GetKey().GetUser())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "list group admins: %v", err)
			}
			admins = append(admins, adminID)
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			return admins, nil
		}
	}
}

func isCaller(ctx context.Context, identityID uuid.UUID) bool {
	caller, err := callerFromContext(ctx)
	return err == nil && caller.id == identityID
}

func organizationObject(id uuid.UUID) string {
	return fmt.Sprintf("%s%s", organizationObjectPrefix, id.String())
}

func groupObject(id uuid.UUID) string {
	return fmt.Sprintf("%s%s", groupObjectPrefix, id.String())
}

func identityObject(id uuid.UUID) string {
	return fmt.Sprintf("%s%s", identityObjectPrefix, id.String())
}

func parseIdentityObject(value string) (uuid.UUID, error) {
	identityID, ok := strings.CutPrefix(value, identityObjectPrefix)
	if !ok {
		return uuid.Nil, fmt.Errorf("unexpected identity object %q", value)
	}
	id, err := uuid.Parse(identityID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("identity object %q: %w", value, err)
	}
	return id, nil
}
