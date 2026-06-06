package server

import (
	"fmt"
	"strings"
)

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must be provided")
	}
	return nil
}
