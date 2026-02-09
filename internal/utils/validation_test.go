package utils_test

import (
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{email: "user@example.com", valid: true},
		{email: "first.last+tag@sub.domain.co", valid: true},
		{email: "missingatsign.com", valid: false},
		{email: "invalid@", valid: false},
		{email: "@domain.com", valid: false},
		{email: "user@domain", valid: false},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.valid, utils.IsValidEmail(tc.email), tc.email)
	}
}
