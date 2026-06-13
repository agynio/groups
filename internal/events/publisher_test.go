package events

import (
	"context"
	"testing"
	"time"

	groupsv1 "github.com/agynio/groups/.gen/go/agynio/api/groups/v1"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type fakeJetStream struct {
	msg      *nats.Msg
	optCount int
}

func (s *fakeJetStream) PublishMsg(msg *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error) {
	s.msg = msg
	s.optCount = len(opts)
	return &nats.PubAck{}, nil
}

func TestPublisherSetsRequiredHeaders(t *testing.T) {
	js := &fakeJetStream{}
	publisher := NewJetStreamPublisher(js)
	occurredAt := time.Date(2026, 6, 6, 23, 0, 0, 123, time.UTC)
	publisher.now = func() time.Time { return occurredAt }
	eventID := uuid.New()
	event := &groupsv1.GroupMembershipAddedEvent{
		GroupId:    uuid.New().String(),
		MemberType: groupsv1.GroupMemberType_GROUP_MEMBER_TYPE_USER,
		MemberId:   uuid.New().String(),
	}

	err := publisher.PublishMembershipAdded(context.Background(), eventID, event)
	require.NoError(t, err)
	require.Equal(t, SubjectMembershipAdded, js.msg.Subject)
	require.Equal(t, eventID.String(), js.msg.Header.Get(HeaderMessageID))
	require.Equal(t, eventID.String(), js.msg.Header.Get(HeaderEventID))
	require.Equal(t, occurredAt.Format(time.RFC3339Nano), js.msg.Header.Get(HeaderOccurredAt))
	require.Equal(t, Producer, js.msg.Header.Get(HeaderProducer))
	require.Equal(t, string(event.ProtoReflect().Descriptor().FullName()), js.msg.Header.Get(HeaderSchema))
	require.Positive(t, js.optCount)

	var decoded groupsv1.GroupMembershipAddedEvent
	require.NoError(t, proto.Unmarshal(js.msg.Data, &decoded))
	require.Equal(t, event.GetGroupId(), decoded.GetGroupId())
}
