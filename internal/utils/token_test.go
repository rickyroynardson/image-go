package utils

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenerateAndValidateJWT(t *testing.T) {
	id := uuid.New()
	token, err := GenerateJWT(id, "secret")
	assert.NotNil(t, token)
	assert.Nil(t, err)

	uuid, err := ValidateJWT(token, "secret")
	assert.Nil(t, err)
	assert.NotNil(t, uuid)
	assert.Equal(t, id, uuid)
}

func TestGenerateRefreshToken(t *testing.T) {
	token, err := GenerateRefresh()
	assert.NotNil(t, token)
	assert.Nil(t, err)
}

func TestGetAuthorizationToken(t *testing.T) {
	tests := []struct {
		name          string
		headers       http.Header
		expectedToken string
		expectedError bool
	}{
		{
			name:          "no authorization header",
			headers:       http.Header{},
			expectedError: true,
		},
		{
			name: "invalid header format",
			headers: http.Header{
				"Authorization": []string{"invalid-token"},
			},
			expectedError: true,
		},
		{
			name: "invalid token",
			headers: http.Header{
				"Authorization": []string{"Bearer "},
			},
			expectedError: true,
		},
		{
			name: "valid token",
			headers: http.Header{
				"Authorization": []string{"Bearer token"},
			},
			expectedToken: "token",
			expectedError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := GetAuthorizationToken(test.headers)
			if test.expectedError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !test.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if test.expectedToken != token {
				t.Errorf("expected token %s, got %s", test.expectedToken, token)
			}
		})
	}
}
