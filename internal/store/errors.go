package store

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type NotFoundError struct {
	Resource string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found", e.Resource)
}

type AlreadyExistsError struct {
	Resource string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s already exists", e.Resource)
}

type InvalidPageTokenError struct {
	Err error
}

func (e *InvalidPageTokenError) Error() string {
	return fmt.Sprintf("invalid page token: %v", e.Err)
}

func (e *InvalidPageTokenError) Unwrap() error {
	return e.Err
}

func NotFound(resource string) error {
	return &NotFoundError{Resource: resource}
}

func AlreadyExists(resource string) error {
	return &AlreadyExistsError{Resource: resource}
}

func InvalidPageToken(err error) error {
	return &InvalidPageTokenError{Err: err}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}
