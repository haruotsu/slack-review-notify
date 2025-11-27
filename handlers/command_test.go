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
			name:     "通常のスペースなしラベル名",
			input:    "needs-review set-mention @user",
			expected: []string{"needs-review", "set-mention", "@user"},
		},
		{
			name:     "ダブルクォートで囲まれたスペース付きラベル名",
			input:    "\"needs review\" set-mention @user",
			expected: []string{"needs review", "set-mention", "@user"},
		},
		{
			name:     "シングルクォートで囲まれたスペース付きラベル名",
			input:    "'security review' add-reviewer @security",
			expected: []string{"security review", "add-reviewer", "@security"},
		},
		{
			name:     "複数のスペース付きパラメータ",
			input:    "\"needs review\" set-mention \"@team lead\"",
			expected: []string{"needs review", "set-mention", "@team lead"},
		},
		{
			name:     "異なるクォート文字の混在",
			input:    "'label name' \"param value\" test",
			expected: []string{"label name", "param value", "test"},
		},
		{
			name:     "クォート内にクォート文字",
			input:    "\"label's name\" 'param \"value\"' test",
			expected: []string{"label's name", "param \"value\"", "test"},
		},
		{
			name:     "空文字列",
			input:    "",
			expected: nil,
		},
		{
			name:     "スペースのみ",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "単一の要素",
			input:    "show",
			expected: []string{"show"},
		},
		{
			name:     "クォートが閉じられていない場合（ダブルクォート）",
			input:    "\"needs review set-mention @user",
			expected: []string{"needs review set-mention @user"},
		},
		{
			name:     "クォートが閉じられていない場合（シングルクォート）",
			input:    "'security review add-reviewer @security",
			expected: []string{"security review add-reviewer @security"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)

			// nilと空のスライスを同等として扱う
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
