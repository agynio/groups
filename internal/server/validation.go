package server

import (
	"fmt"
	"regexp"
	"strings"
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
