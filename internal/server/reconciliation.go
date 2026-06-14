package server

import (
	"context"
	"log"
	"strings"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const reconciliationPageSize = 100

type ReconciliationReport struct {
	GroupsScanned              int
	MembershipsScanned         int
	TuplesWritten              int
	TuplesDeleted              int
	OrphanedMembershipsRemoved int
}

func (s *Server) Reconcile(ctx context.Context) (ReconciliationReport, error) {
	groups, err := s.store.ListAllGroups(ctx)
	if err != nil {
		return ReconciliationReport{}, status.Errorf(codes.Internal, "list groups: %v", err)
	}
	memberships, err := s.store.ListAllMemberships(ctx)
	if err != nil {
		return ReconciliationReport{}, status.Errorf(codes.Internal, "list memberships: %v", err)
	}

	state := newReconciliationState(groups, memberships)
	report := ReconciliationReport{GroupsScanned: len(groups), MembershipsScanned: len(memberships)}
	for _, group := range groups {
		written, err := s.ensureTuple(ctx, groupOrgTuple(group.Meta.ID, group.OrganizationID))
		if err != nil {
			return ReconciliationReport{}, status.Errorf(codes.Internal, "repair group org tuple: %v", err)
		}
		if written {
			report.TuplesWritten++
		}
	}

	for _, membership := range memberships {
		if _, ok := state.groups[membership.GroupID]; !ok {
			panic("membership references missing group")
		}
		exists, err := s.identityExists(ctx, membership.MemberID)
		if err != nil {
			return ReconciliationReport{}, status.Errorf(codes.Internal, "lookup membership identity: %v", err)
		}
		if !exists {
			removed, deleted, err := s.store.RemoveMember(ctx, membership.GroupID, membership.MemberID)
			if err != nil {
				return ReconciliationReport{}, status.Errorf(codes.Internal, "remove orphaned membership: %v", err)
			}
			if deleted {
				report.OrphanedMembershipsRemoved++
				state.removeMembership(removed.Membership)
				deletedTuple, err := s.deleteTupleIfPresent(ctx, membershipTuple(removed.Membership))
				if err != nil {
					return ReconciliationReport{}, status.Errorf(codes.Internal, "delete orphaned membership tuple: %v", err)
				}
				if deletedTuple {
					report.TuplesDeleted++
				}
				s.publishReconciledMembershipRemoval(ctx, removed)
			}
			continue
		}
		written, err := s.ensureTuple(ctx, membershipTuple(membership))
		if err != nil {
			return ReconciliationReport{}, status.Errorf(codes.Internal, "repair membership tuple: %v", err)
		}
		if written {
			report.TuplesWritten++
		}
	}

	deleted, err := s.deleteStaleGroupTuples(ctx, state)
	if err != nil {
		return ReconciliationReport{}, err
	}
	report.TuplesDeleted += deleted
	return report, nil
}

type reconciliationState struct {
	groups      map[uuid.UUID]store.Group
	memberships map[string]store.GroupMembership
}

func newReconciliationState(groups []store.Group, memberships []store.GroupMembership) reconciliationState {
	state := reconciliationState{
		groups:      make(map[uuid.UUID]store.Group, len(groups)),
		memberships: make(map[string]store.GroupMembership, len(memberships)),
	}
	for _, group := range groups {
		state.groups[group.Meta.ID] = group
	}
	for _, membership := range memberships {
		state.memberships[tupleKeyString(membershipTuple(membership))] = membership
	}
	return state
}

func (s *Server) publishReconciledMembershipRemoval(ctx context.Context, removed store.RemovedMembership) {
	if err := s.publisher.PublishMembershipRemoved(ctx, uuid.New(), &groupsv1.GroupMembershipRemovedEvent{
		GroupId:    removed.Membership.GroupID.String(),
		MemberType: toProtoMemberType(removed.Membership.MemberType),
		MemberId:   removed.Membership.MemberID.String(),
	}); err != nil {
		log.Printf("publish membership removed failed during reconciliation (group=%s member=%s org=%s): %v", removed.Membership.GroupID, removed.Membership.MemberID, removed.OrganizationID, err)
	}
	s.notifyMembershipUpdated(ctx, removed.OrganizationID.String(), removed.Membership.GroupID.String(), removed.Membership.MemberID.String())
}

func (state *reconciliationState) removeMembership(membership store.GroupMembership) {
	delete(state.memberships, tupleKeyString(membershipTuple(membership)))
}

func (s *Server) ensureTuple(ctx context.Context, tuple *authorizationv1.TupleKey) (bool, error) {
	exists, err := s.tupleExists(ctx, tuple)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	_, err = s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Writes: []*authorizationv1.TupleKey{tuple}})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Server) deleteTupleIfPresent(ctx context.Context, tuple *authorizationv1.TupleKey) (bool, error) {
	exists, err := s.tupleExists(ctx, tuple)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	_, err = s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Deletes: []*authorizationv1.TupleKey{tuple}})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Server) tupleExists(ctx context.Context, tuple *authorizationv1.TupleKey) (bool, error) {
	tuples, err := s.readTuples(ctx, tuple)
	if err != nil {
		return false, err
	}
	for _, candidate := range tuples {
		if tupleKeyString(candidate) == tupleKeyString(tuple) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) readTuples(ctx context.Context, tuple *authorizationv1.TupleKey) ([]*authorizationv1.TupleKey, error) {
	pageToken := ""
	tuples := []*authorizationv1.TupleKey{}
	for {
		response, err := s.authorizationClient.Read(ctx, &authorizationv1.ReadRequest{
			TupleKey:  tuple,
			PageSize:  reconciliationPageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, err
		}
		for _, result := range response.GetTuples() {
			tuples = append(tuples, result.GetKey())
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			return tuples, nil
		}
	}
}

func (s *Server) identityExists(ctx context.Context, identityID uuid.UUID) (bool, error) {
	_, err := s.identityClient.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: identityID.String()})
	if err == nil {
		return true, nil
	}
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}

func (s *Server) deleteStaleGroupTuples(ctx context.Context, state reconciliationState) (int, error) {
	tuples, err := s.readTuples(ctx, &authorizationv1.TupleKey{})
	if err != nil {
		return 0, status.Errorf(codes.Internal, "read tuples: %v", err)
	}
	deletes := []*authorizationv1.TupleKey{}
	for _, tuple := range tuples {
		if !isManagedGroupTuple(tuple) {
			continue
		}
		if !state.tupleBackedByStore(tuple) {
			deletes = append(deletes, tuple)
		}
	}
	if len(deletes) == 0 {
		return 0, nil
	}
	_, err = s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{Deletes: deletes})
	if err != nil {
		return 0, status.Errorf(codes.Internal, "delete stale tuples: %v", err)
	}
	return len(deletes), nil
}

func isManagedGroupTuple(tuple *authorizationv1.TupleKey) bool {
	if tuple.GetRelation() == orgRelation && hasGroupObjectPrefix(tuple.GetUser()) && hasOrganizationObjectPrefix(tuple.GetObject()) {
		return true
	}
	if tuple.GetRelation() == memberRelation && hasIdentityObjectPrefix(tuple.GetUser()) && hasGroupObjectPrefix(tuple.GetObject()) {
		return true
	}
	return false
}

func (state reconciliationState) tupleBackedByStore(tuple *authorizationv1.TupleKey) bool {
	switch tuple.GetRelation() {
	case orgRelation:
		groupID, err := parseGroupObject(tuple.GetUser())
		if err != nil {
			return false
		}
		group, ok := state.groups[groupID]
		return ok && tuple.GetObject() == organizationObject(group.OrganizationID)
	case memberRelation:
		_, ok := state.memberships[tupleKeyString(tuple)]
		return ok
	default:
		panic("unexpected managed group tuple relation")
	}
}

func hasGroupObjectPrefix(value string) bool {
	_, ok := strings.CutPrefix(value, groupObjectPrefix)
	return ok
}

func hasIdentityObjectPrefix(value string) bool {
	_, ok := strings.CutPrefix(value, identityObjectPrefix)
	return ok
}

func hasOrganizationObjectPrefix(value string) bool {
	_, ok := strings.CutPrefix(value, organizationObjectPrefix)
	return ok
}

func parseGroupObject(value string) (uuid.UUID, error) {
	groupID, ok := strings.CutPrefix(value, groupObjectPrefix)
	if !ok {
		return uuid.Nil, status.Errorf(codes.Internal, "unexpected group object %q", value)
	}
	id, err := uuid.Parse(groupID)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.Internal, "group object %q: %v", value, err)
	}
	return id, nil
}

func tupleKeyString(tuple *authorizationv1.TupleKey) string {
	return tuple.GetUser() + "|" + tuple.GetRelation() + "|" + tuple.GetObject()
}
