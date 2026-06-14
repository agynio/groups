package server

import (
	"context"
	"testing"
	"time"

	authorizationv1 "github.com/agynio/groups/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReconcilerRunExecutesReconciliation(t *testing.T) {
	groupStore := newFakeStore()
	organizationID := uuid.New()
	groupID := uuid.New()
	groupStore.groups[groupID] = store.Group{
		Meta:           store.EntityMeta{ID: groupID, CreatedAt: groupStore.now, UpdatedAt: groupStore.now},
		OrganizationID: organizationID,
		Name:           "engineering",
		Source:         store.GroupSourcePlatform,
	}
	auth := &fakeAuthorizationClient{}
	reconciled := make(chan struct{}, 1)
	auth.writeHook = func(req *authorizationv1.WriteRequest) {
		select {
		case reconciled <- struct{}{}:
		default:
		}
	}
	server := New(groupStore, auth, &fakeIdentityClient{types: map[string]identityv1.IdentityType{}}, &fakePublisher{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go NewReconciler(server, time.Hour).Run(ctx)

	select {
	case <-reconciled:
	case <-time.After(time.Second):
		require.Fail(t, "reconciler did not run")
	}
}
