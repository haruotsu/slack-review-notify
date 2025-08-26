package services

import (
	"testing"

	"github.com/google/go-github/v71/github"
	"github.com/stretchr/testify/assert"
)

func TestGetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		user     *github.User
		expected string
	}{
		{
			name: "Name が設定されている場合は Name を返す",
			user: &github.User{
				Login: github.Ptr("testuser"),
				Name:  github.Ptr("Test User"),
			},
			expected: "Test User",
		},
		{
			name: "Name が空文字の場合は Login を返す",
			user: &github.User{
				Login: github.Ptr("testuser"),
				Name:  github.Ptr(""),
			},
			expected: "testuser",
		},
		{
			name: "Name が nil の場合は Login を返す",
			user: &github.User{
				Login: github.Ptr("testuser"),
				Name:  nil,
			},
			expected: "testuser",
		},
		{
			name: "Login も nil の場合は空文字を返す",
			user: &github.User{
				Login: nil,
				Name:  nil,
			},
			expected: "",
		},
		{
			name: "user が nil の場合は空文字を返す",
			user: nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDisplayName(tt.user)
			assert.Equal(t, tt.expected, result)
		})
	}
}