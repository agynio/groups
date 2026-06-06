package server

import (
	"context"
	"fmt"

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
)

func (s *Server) writeGroupOrgTuple(ctx context.Context, groupID uuid.UUID, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{{
			User:     fmt.Sprintf("%s%s", groupObjectPrefix, groupID.String()),
			Relation: orgRelation,
			Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
		}},
	})
	return err
}

func (s *Server) deleteGroupOrgTuple(ctx context.Context, groupID uuid.UUID, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Deletes: []*authorizationv1.TupleKey{{
			User:     fmt.Sprintf("%s%s", groupObjectPrefix, groupID.String()),
			Relation: orgRelation,
			Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
		}},
	})
	return err
}

func (s *Server) writeMembershipTuple(ctx context.Context, membership store.GroupMembership) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{{
			User:     fmt.Sprintf("%s%s", identityObjectPrefix, membership.MemberID.String()),
			Relation: memberRelation,
			Object:   fmt.Sprintf("%s%s", groupObjectPrefix, membership.GroupID.String()),
		}},
	})
	return err
}

func (s *Server) deleteMembershipTuple(ctx context.Context, membership store.GroupMembership) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Deletes: []*authorizationv1.TupleKey{{
			User:     fmt.Sprintf("%s%s", identityObjectPrefix, membership.MemberID.String()),
			Relation: memberRelation,
			Object:   fmt.Sprintf("%s%s", groupObjectPrefix, membership.GroupID.String()),
		}},
	})
	return err
}
