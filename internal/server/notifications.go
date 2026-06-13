package server

import (
	"context"
	"fmt"
	"log"

	notificationsv1 "github.com/agynio/groups/.gen/go/agynio/api/notifications/v1"
	"github.com/agynio/groups/internal/store"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	notificationEventGroupUpdated      = "group.updated"
	notificationEventMembershipUpdated = "group_membership.updated"
	notificationSource                 = "groups-service"
)

func (s *Server) notifyGroupUpdated(ctx context.Context, group store.Group) {
	if s.notificationsClient == nil {
		return
	}
	payload, err := structpb.NewStruct(map[string]any{"group_id": group.Meta.ID.String(), "organization_id": group.OrganizationID.String()})
	if err != nil {
		panic(fmt.Sprintf("build group notification payload: %v", err))
	}
	_, err = s.notificationsClient.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   notificationEventGroupUpdated,
		Rooms:   []string{organizationRoom(group.OrganizationID.String())},
		Payload: payload,
		Source:  notificationSource,
	})
	if err != nil {
		log.Printf("publish group notification failed (group=%s org=%s): %v", group.Meta.ID, group.OrganizationID, err)
	}
}

func (s *Server) notifyMembershipUpdated(ctx context.Context, organizationID string, groupID string, memberID string) {
	if s.notificationsClient == nil {
		return
	}
	payload, err := structpb.NewStruct(map[string]any{"group_id": groupID, "member_id": memberID, "organization_id": organizationID})
	if err != nil {
		panic(fmt.Sprintf("build group membership notification payload: %v", err))
	}
	_, err = s.notificationsClient.Publish(ctx, &notificationsv1.PublishRequest{
		Event:   notificationEventMembershipUpdated,
		Rooms:   []string{organizationRoom(organizationID)},
		Payload: payload,
		Source:  notificationSource,
	})
	if err != nil {
		log.Printf("publish group membership notification failed (group=%s member=%s org=%s): %v", groupID, memberID, organizationID, err)
	}
}

func organizationRoom(organizationID string) string {
	return fmt.Sprintf("organization:%s", organizationID)
}
