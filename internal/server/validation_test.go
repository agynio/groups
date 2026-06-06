package server

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "lowercase", value: "engineering", wantErr: false},
		{name: "underscore", value: "eng_team", wantErr: false},
		{name: "hyphen", value: "eng-team", wantErr: false},
		{name: "digits", value: "eng2", wantErr: false},
		{name: "blank", value: "", wantErr: true},
		{name: "whitespace", value: "  ", wantErr: true},
		{name: "uppercase", value: "Engineering", wantErr: true},
		{name: "space", value: "eng team", wantErr: true},
		{name: "invalid character", value: "eng.team", wantErr: true},
		{name: "over length", value: strings.Repeat("a", maxGroupNameLength+1), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateName(test.value)
			if test.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
