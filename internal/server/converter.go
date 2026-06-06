package server

import (
	"fmt"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	identityv1 "github.com/agynio/groups/.gen/go/agynio/api/identity/v1"
	"github.com/agynio/groups/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoEntityMeta(meta store.EntityMeta) *groupsv1.EntityMeta {
	return &groupsv1.EntityMeta{
		Id:        meta.ID.String(),
		CreatedAt: timestamppb.New(meta.CreatedAt),
		UpdatedAt: timestamppb.New(meta.UpdatedAt),
	}
}

func toProtoGroup(group store.Group) *groupsv1.Group {
	protoGroup := &groupsv1.Group{
		Meta:           toProtoEntityMeta(group.Meta),
		OrganizationId: group.OrganizationID.String(),
		Name:           group.Name,
		Description:    group.Description,
		Source:         toProtoGroupSource(group.Source),
	}
	if group.ExternalID != nil {
		externalID := *group.ExternalID
		protoGroup.ExternalId = &externalID
	}
	return protoGroup
}

func toProtoMembership(membership store.GroupMembership) *groupsv1.GroupMembership {
	return &groupsv1.GroupMembership{
		Meta:       toProtoEntityMeta(membership.Meta),
		GroupId:    membership.GroupID.String(),
		MemberType: toProtoMemberType(membership.MemberType),
		MemberId:   membership.MemberID.String(),
		Source:     toProtoGroupSource(membership.Source),
	}
}

func toProtoGroupSource(source store.GroupSource) groupsv1.GroupSource {
	switch source {
	case store.GroupSourcePlatform:
		return groupsv1.GroupSource_GROUP_SOURCE_PLATFORM
	case store.GroupSourceSCIM:
		return groupsv1.GroupSource_GROUP_SOURCE_SCIM
	default:
		panic(fmt.Sprintf("unexpected group source: %q", source))
	}
}

func toStoreGroupSource(source groupsv1.GroupSource) (store.GroupSource, error) {
	switch source {
	case groupsv1.GroupSource_GROUP_SOURCE_PLATFORM:
		return store.GroupSourcePlatform, nil
	case groupsv1.GroupSource_GROUP_SOURCE_SCIM:
		return store.GroupSourceSCIM, nil
	default:
		return "", fmt.Errorf("unknown group source %v", source)
	}
}

func toProtoMemberType(memberType store.GroupMemberType) groupsv1.GroupMemberType {
	switch memberType {
	case store.GroupMemberTypeUser:
		return groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER
	case store.GroupMemberTypeAgent:
		return groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_AGENT
	case store.GroupMemberTypeApp:
		return groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_APP
	default:
		panic(fmt.Sprintf("unexpected member type: %q", memberType))
	}
}

func toStoreMemberType(memberType groupsv1.GroupMemberType) (store.GroupMemberType, error) {
	switch memberType {
	case groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER:
		return store.GroupMemberTypeUser, nil
	case groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_AGENT:
		return store.GroupMemberTypeAgent, nil
	case groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_APP:
		return store.GroupMemberTypeApp, nil
	default:
		return "", fmt.Errorf("unknown member type %v", memberType)
	}
}

func expectedIdentityType(memberType store.GroupMemberType) identityv1.IdentityType {
	switch memberType {
	case store.GroupMemberTypeUser:
		return identityv1.IdentityType_IDENTITY_TYPE_USER
	case store.GroupMemberTypeAgent:
		return identityv1.IdentityType_IDENTITY_TYPE_AGENT
	case store.GroupMemberTypeApp:
		return identityv1.IdentityType_IDENTITY_TYPE_APP
	default:
		panic(fmt.Sprintf("unexpected member type: %q", memberType))
	}
}
