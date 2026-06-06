package server

import (
	"errors"
	"fmt"

	"github.com/agynio/groups/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("must be a valid UUID")
	}
	return id, nil
}

func decodeCursor(token string) (*store.PageCursor, error) {
	if token == "" {
		return nil, nil
	}
	id, err := store.DecodePageToken(token)
	if err != nil {
		return nil, store.InvalidPageToken(err)
	}
	return &store.PageCursor{AfterID: id}, nil
}

func encodeCursor(cursor *store.PageCursor) string {
	if cursor == nil {
		return ""
	}
	return store.EncodePageToken(cursor.AfterID)
}

func toStatusError(err error) error {
	var notFound *store.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	var alreadyExists *store.AlreadyExistsError
	if errors.As(err, &alreadyExists) {
		return status.Error(codes.AlreadyExists, err.Error())
	}
	var invalidPageToken *store.InvalidPageTokenError
	if errors.As(err, &invalidPageToken) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
