package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPageTokenRoundTrip(t *testing.T) {
	id := uuid.New()
	decoded, err := DecodePageToken(EncodePageToken(id))
	require.NoError(t, err)
	require.Equal(t, id, decoded)
}

func TestNormalizePageSize(t *testing.T) {
	require.Equal(t, DefaultListPageSize, NormalizePageSize(0))
	require.Equal(t, int32(10), NormalizePageSize(10))
	require.Equal(t, MaxListPageSize, NormalizePageSize(MaxListPageSize+1))
}
