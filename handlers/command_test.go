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
