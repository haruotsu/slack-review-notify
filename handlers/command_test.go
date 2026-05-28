package handlers

import (
	"reflect"
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Normal label name without spaces",
			input:    "needs-review set-mention @user",
			expected: []string{"needs-review", "set-mention", "@user"},
		},
		{
			name:     "Label name with spaces enclosed in double quotes",
			input:    "\"needs review\" set-mention @user",
			expected: []string{"needs review", "set-mention", "@user"},
		},
		{
			name:     "Label name with spaces enclosed in single quotes",
			input:    "'security review' add-reviewer @security",
			expected: []string{"security review", "add-reviewer", "@security"},
		},
		{
			name:     "Multiple parameters with spaces",
			input:    "\"needs review\" set-mention \"@team lead\"",
			expected: []string{"needs review", "set-mention", "@team lead"},
		},
		{
			name:     "Mixed different quote characters",
			input:    "'label name' \"param value\" test",
			expected: []string{"label name", "param value", "test"},
		},
		{
			name:     "Quote characters inside quotes",
			input:    "\"label's name\" 'param \"value\"' test",
			expected: []string{"label's name", "param \"value\"", "test"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "Spaces only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "Single element",
			input:    "show",
			expected: []string{"show"},
		},
		{
			name:     "Unclosed quote (double quote)",
			input:    "\"needs review set-mention @user",
			expected: []string{"needs review set-mention @user"},
		},
		{
			name:     "Unclosed quote (single quote)",
			input:    "'security review add-reviewer @security",
			expected: []string{"security review add-reviewer @security"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)

			// Treat nil and empty slices as equivalent
			if tt.expected == nil && len(result) == 0 {
				return
			}
			if len(tt.expected) == 0 && len(result) == 0 {
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseCommand(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCleanUserID covers the input shapes the Slack slash command can deliver.
// `<@U…|name>` is the form Slack uses when "Escape channels, users, and links"
// is enabled on the slash command, and the prior implementation accidentally
// stored `U…|name` as the slack user id.
func TestCleanUserID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "bare user id", input: "U01ABCDE234", expected: "U01ABCDE234"},
		{name: "wrapped user id", input: "<@U01ABCDE234>", expected: "U01ABCDE234"},
		{name: "wrapped user id with display name", input: "<@U01ABCDE234|octocat>", expected: "U01ABCDE234"},
		{name: "subteam mention", input: "<!subteam^S0123|@team>", expected: "S0123"},
		{name: "plain at handle", input: "@octocat", expected: "octocat"},
		{name: "whitespace", input: "  U01ABCDE234  ", expected: "U01ABCDE234"},
		{name: "comma stripped", input: ",U01ABCDE234,", expected: "U01ABCDE234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanUserID(tt.input)
			if got != tt.expected {
				t.Errorf("cleanUserID(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestLooksLikeSlackUserID verifies the helper used to validate `set-user`
// input. Anything starting with U/W followed by uppercase alphanumerics counts
// as a Slack user id. A bare github-style handle must not pass.
func TestLooksLikeSlackUserID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "U-prefixed id", input: "U01ABCDE234", want: true},
		{name: "W-prefixed enterprise id", input: "W01ABCDE234", want: true},
		{name: "lowercase plain", input: "octocat", want: false},
		{name: "U-prefixed but with display suffix", input: "U01ABCDE234|octocat", want: false},
		{name: "empty", input: "", want: false},
		{name: "mention markup", input: "<@U01ABCDE234>", want: false},
		{name: "subteam id", input: "S0123ABCDEF", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeSlackUserID(tt.input)
			if got != tt.want {
				t.Errorf("looksLikeSlackUserID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
