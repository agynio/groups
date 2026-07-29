package server

import (
	"context"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
)

const (
	organizationObjectPrefix = "organization:"
	identityObjectPrefix     = "identity:"
	groupObjectPrefix        = "group:"
	orgRelation              = "org"
	memberRelation           = "member"
	adminRelation            = "admin"
)

func (s *Server) writeGroupCreateTuples(ctx context.Context, groupID uuid.UUID, organizationID uuid.UUID, adminID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Writes: []*authorizationv1.TupleKey{
		groupOrgTuple(groupID, organizationID),
		groupAdminTuple(groupID, adminID),
	}})
	return err
}

func (s *Server) deleteGroupTuples(ctx context.Context, group store.DeletedGroup) error {
	deletes := make([]*authorizationv1.TupleKey, 0, len(group.Memberships)+1+len(group.Admins))
	deletes = append(deletes, groupOrgTuple(group.Group.Meta.ID, group.Group.OrganizationID))
	for _, adminID := range group.Admins {
		deletes = append(deletes, groupAdminTuple(group.Group.Meta.ID, adminID))
	}
	for _, membership := range group.Memberships {
		deletes = append(deletes, membershipTuple(membership))
	}
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Deletes: deletes})
	return err
}

func (s *Server) writeMembershipTuple(ctx context.Context, membership store.GroupMembership) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Writes: []*authorizationv1.TupleKey{membershipTuple(membership)}})
	return err
}

func (s *Server) deleteMembershipTuple(ctx context.Context, membership store.GroupMembership) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Deletes: []*authorizationv1.TupleKey{membershipTuple(membership)}})
	return err
}

// The group holds the relation, so the group is the object: the model defines
// `org: [organization]` on group, not on organization. Writing it the other way
// round names a relation organization does not have, and OpenFGA rejects the
// whole Write — which took group creation down with it.
func groupOrgTuple(groupID uuid.UUID, organizationID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{User: organizationObject(organizationID), Relation: orgRelation, Object: groupObject(groupID)}
}

func groupAdminTuple(groupID uuid.UUID, adminID uuid.UUID) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{User: identityObject(adminID), Relation: adminRelation, Object: groupObject(groupID)}
}

func membershipTuple(membership store.GroupMembership) *authorizationv1.TupleKey {
	return &authorizationv1.TupleKey{User: identityObject(membership.MemberID), Relation: memberRelation, Object: groupObject(membership.GroupID)}
}
