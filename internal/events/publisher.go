package events

import (
	"context"
	"fmt"
	"time"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const (
	SubjectMembershipAdded   = "agyn.groups.membership.added"
	SubjectMembershipRemoved = "agyn.groups.membership.removed"
	SubjectGroupDeleted      = "agyn.groups.group.deleted"

	HeaderMessageID  = "Nats-Msg-Id"
	HeaderEventID    = "Agyn-Event-Id"
	HeaderOccurredAt = "Agyn-Occurred-At"
	HeaderProducer   = "Agyn-Producer"
	HeaderSchema     = "Agyn-Schema"
	Producer         = "groups-service"
)

type Publisher interface {
	PublishMembershipAdded(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipAddedEvent) error
	PublishMembershipRemoved(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipRemovedEvent) error
	PublishGroupDeleted(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupDeletedEvent) error
}

type jetStream interface {
	PublishMsg(m *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error)
}

type NATSJetStreamPublisher struct {
	js  jetStream
	now func() time.Time
}

func NewNATSJetStreamPublisher(conn *nats.Conn) (NATSJetStreamPublisher, error) {
	js, err := conn.JetStream()
	if err != nil {
		return NATSJetStreamPublisher{}, fmt.Errorf("create jetstream context: %w", err)
	}
	return NewJetStreamPublisher(js), nil
}

func NewJetStreamPublisher(js jetStream) NATSJetStreamPublisher {
	return NATSJetStreamPublisher{js: js, now: time.Now}
}

func (p NATSJetStreamPublisher) PublishMembershipAdded(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipAddedEvent) error {
	return p.publish(ctx, SubjectMembershipAdded, eventID, string(event.ProtoReflect().Descriptor().FullName()), event)
}

func (p NATSJetStreamPublisher) PublishMembershipRemoved(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupMembershipRemovedEvent) error {
	return p.publish(ctx, SubjectMembershipRemoved, eventID, string(event.ProtoReflect().Descriptor().FullName()), event)
}

func (p NATSJetStreamPublisher) PublishGroupDeleted(ctx context.Context, eventID uuid.UUID, event *groupsv1.GroupDeletedEvent) error {
	return p.publish(ctx, SubjectGroupDeleted, eventID, string(event.ProtoReflect().Descriptor().FullName()), event)
}

func (p NATSJetStreamPublisher) publish(ctx context.Context, subject string, eventID uuid.UUID, schema string, message proto.Message) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	ack, err := p.js.PublishMsg(&nats.Msg{
		Subject: subject,
		Header:  eventHeaders(eventID, p.now(), schema),
		Data:    payload,
	}, nats.Context(ctx), nats.MsgId(eventID.String()))
	if err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	if ack == nil {
		return fmt.Errorf("publish %s: missing acknowledgement", subject)
	}
	return nil
}

func eventHeaders(eventID uuid.UUID, occurredAt time.Time, schema string) nats.Header {
	eventIDValue := eventID.String()
	return nats.Header{
		HeaderMessageID:  []string{eventIDValue},
		HeaderEventID:    []string{eventIDValue},
		HeaderOccurredAt: []string{occurredAt.UTC().Format(time.RFC3339Nano)},
		HeaderProducer:   []string{Producer},
		HeaderSchema:     []string{schema},
	}
}

type NoopPublisher struct{}

func (NoopPublisher) PublishMembershipAdded(context.Context, uuid.UUID, *groupsv1.GroupMembershipAddedEvent) error {
	return nil
}

func (NoopPublisher) PublishMembershipRemoved(context.Context, uuid.UUID, *groupsv1.GroupMembershipRemovedEvent) error {
	return nil
}

func (NoopPublisher) PublishGroupDeleted(context.Context, uuid.UUID, *groupsv1.GroupDeletedEvent) error {
	return nil
}

func Connect(url string) (*nats.Conn, error) {
	conn, err := nats.Connect(url, nats.Timeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}
	return conn, nil
}
