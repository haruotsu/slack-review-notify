package handlers

import (
	"reflect"
	"testing"
	"time"
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

func TestParseAwayPeriod_HalfDay(t *testing.T) {
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, jst)

	tests := []struct {
		name          string
		parts         []string
		wantFromHour  int
		wantUntilHour int
		wantUntilDay  int
		wantLeaveType string
		wantReason    string
		wantErr       string
	}{
		{
			name:          "on date am",
			parts:         []string{"@user", "on", "2026-08-05", "am"},
			wantFromHour:  6,
			wantUntilHour: 14,
			wantUntilDay:  5,
			wantLeaveType: "am",
		},
		{
			name:          "on date pm",
			parts:         []string{"@user", "on", "2026-08-05", "pm"},
			wantFromHour:  14,
			wantUntilHour: 0,
			wantUntilDay:  6,
			wantLeaveType: "pm",
		},
		{
			name:          "on date without modifier (full day)",
			parts:         []string{"@user", "on", "2026-08-05"},
			wantFromHour:  0,
			wantUntilHour: 23,
			wantUntilDay:  5,
			wantLeaveType: "",
		},
		{
			name:          "on date am reason text",
			parts:         []string{"@user", "on", "2026-08-05", "am", "reason", "有給休暇"},
			wantFromHour:  6,
			wantUntilHour: 14,
			wantUntilDay:  5,
			wantLeaveType: "am",
			wantReason:    "有給休暇",
		},
		{
			name:    "on past date am (expired)",
			parts:   []string{"@user", "on", "2026-08-04", "am"},
			wantErr: "cmd.set_away.half_day_expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			period, errKey := parseAwayPeriod(tt.parts, jst, now, rejectPastDates)
			if tt.wantErr != "" {
				if errKey != tt.wantErr {
					t.Errorf("expected error key %q, got %q", tt.wantErr, errKey)
				}
				return
			}
			if errKey != "" {
				t.Fatalf("unexpected error key: %s", errKey)
			}
			if period.leaveType != tt.wantLeaveType {
				t.Errorf("leaveType = %q, want %q", period.leaveType, tt.wantLeaveType)
			}
			if period.from == nil {
				t.Fatal("from is nil")
			}
			if period.from.Hour() != tt.wantFromHour {
				t.Errorf("from.Hour() = %d, want %d", period.from.Hour(), tt.wantFromHour)
			}
			if period.until == nil {
				t.Fatal("until is nil")
			}
			if period.until.Hour() != tt.wantUntilHour {
				t.Errorf("until.Hour() = %d, want %d", period.until.Hour(), tt.wantUntilHour)
			}
			if period.until.Day() != tt.wantUntilDay {
				t.Errorf("until.Day() = %d, want %d", period.until.Day(), tt.wantUntilDay)
			}
			if period.reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", period.reason, tt.wantReason)
			}
		})
	}
}
