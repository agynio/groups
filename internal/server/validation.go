package server

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/agynio/groups/internal/store"
)

const maxGroupNameLength = 64

var groupNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must be provided")
	}
	if len(name) > maxGroupNameLength {
		return fmt.Errorf("name must be %d characters or less", maxGroupNameLength)
	}
	if !groupNamePattern.MatchString(name) {
		return fmt.Errorf("name must match %s", groupNamePattern.String())
	}
	return nil
}

func validateExternalID(source store.GroupSource, externalID *string) error {
	switch source {
	case store.GroupSourcePlatform:
		if externalID != nil && strings.TrimSpace(*externalID) != "" {
			return fmt.Errorf("must be empty for platform groups")
		}
	case store.GroupSourceSCIM:
		if externalID == nil || strings.TrimSpace(*externalID) == "" {
			return fmt.Errorf("must be provided for scim groups")
		}
	default:
		panic(fmt.Sprintf("unexpected group source: %q", source))
	}
	return nil
}
