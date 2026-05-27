package services

import (
	"slack-review-notify/models"
	"strings"
	"testing"
)

func TestBuildSettingsModalView_BasicShape(t *testing.T) {
	cfg := &models.ChannelConfig{
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U99999",
	}
	view := BuildSettingsModalView("C12345", cfg, "ja")

	if view["type"] != "modal" {
		t.Errorf("type = %v, want modal", view["type"])
	}
	if view["callback_id"] != "settings_modal" {
		t.Errorf("callback_id = %v, want settings_modal", view["callback_id"])
	}
	// private_metadata must contain channel ID so view_submission can find the channel
	pm, ok := view["private_metadata"].(string)
	if !ok || !strings.Contains(pm, "C12345") {
		t.Errorf("private_metadata = %v, want to include C12345", view["private_metadata"])
	}
	// must have submit & close buttons
	if _, ok := view["submit"].(map[string]interface{}); !ok {
		t.Errorf("missing submit button")
	}
	if _, ok := view["close"].(map[string]interface{}); !ok {
		t.Errorf("missing close button")
	}
}

func TestBuildSettingsModalView_HasAllFields(t *testing.T) {
	cfg := &models.ChannelConfig{
		SlackChannelID:           "C12345",
		LabelName:                "needs-review",
		DefaultMentionID:         "U99999",
		ReviewerList:             "U1,U2",
		RepositoryList:           "owner/repo1,owner/repo2",
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
		RequiredApprovals:        2,
		Language:                 "ja",
		IsActive:                 true,
	}
	view := BuildSettingsModalView("C12345", cfg, "ja")

	blocks, ok := view["blocks"].([]map[string]interface{})
	if !ok {
		t.Fatalf("blocks is not a slice: %T", view["blocks"])
	}

	requiredBlockIDs := []string{
		"label_name",
		"default_mention_id",
		"reviewer_list",
		"repository_list",
		"reviewer_reminder_interval",
		"business_hours_start",
		"business_hours_end",
		"timezone",
		"required_approvals",
		"language",
		"is_active",
	}

	found := make(map[string]bool)
	for _, b := range blocks {
		if id, ok := b["block_id"].(string); ok {
			found[id] = true
		}
	}

	for _, want := range requiredBlockIDs {
		if !found[want] {
			t.Errorf("missing block_id: %s", want)
		}
	}
}

func TestBuildSettingsModalView_InitialValues(t *testing.T) {
	cfg := &models.ChannelConfig{
		SlackChannelID:           "C12345",
		LabelName:                "my-label",
		DefaultMentionID:         "U99999",
		ReviewerList:             "U1,U2",
		RepositoryList:           "owner/repo1",
		ReviewerReminderInterval: 45,
		BusinessHoursStart:       "10:00",
		BusinessHoursEnd:         "19:00",
		Timezone:                 "UTC",
		RequiredApprovals:        2,
		Language:                 "en",
		IsActive:                 true,
	}
	view := BuildSettingsModalView("C12345", cfg, "en")
	blocks := view["blocks"].([]map[string]interface{})

	getElement := func(blockID string) map[string]interface{} {
		for _, b := range blocks {
			if id, ok := b["block_id"].(string); ok && id == blockID {
				if el, ok := b["element"].(map[string]interface{}); ok {
					return el
				}
			}
		}
		return nil
	}

	tests := []struct {
		blockID string
		key     string
		want    string
	}{
		{"label_name", "initial_value", "my-label"},
		{"default_mention_id", "initial_value", "U99999"},
		{"reviewer_list", "initial_value", "U1,U2"},
		{"repository_list", "initial_value", "owner/repo1"},
		{"reviewer_reminder_interval", "initial_value", "45"},
		{"business_hours_start", "initial_value", "10:00"},
		{"business_hours_end", "initial_value", "19:00"},
		{"timezone", "initial_value", "UTC"},
		{"required_approvals", "initial_value", "2"},
	}

	for _, tt := range tests {
		el := getElement(tt.blockID)
		if el == nil {
			t.Errorf("element for %s not found", tt.blockID)
			continue
		}
		got, _ := el[tt.key].(string)
		if got != tt.want {
			t.Errorf("%s.%s = %q, want %q", tt.blockID, tt.key, got, tt.want)
		}
	}

	// language: static_select; initial_option must have value "en"
	langEl := getElement("language")
	if langEl == nil {
		t.Fatalf("language element not found")
	}
	opt, ok := langEl["initial_option"].(map[string]interface{})
	if !ok {
		t.Fatalf("language.initial_option missing")
	}
	if opt["value"] != "en" {
		t.Errorf("language.initial_option.value = %v, want en", opt["value"])
	}

	// is_active: static_select; initial_option must reflect IsActive=true
	activeEl := getElement("is_active")
	if activeEl == nil {
		t.Fatalf("is_active element not found")
	}
	activeOpt, ok := activeEl["initial_option"].(map[string]interface{})
	if !ok {
		t.Fatalf("is_active.initial_option missing")
	}
	if activeOpt["value"] != "true" {
		t.Errorf("is_active.initial_option.value = %v, want true", activeOpt["value"])
	}
}

func TestBuildSettingsModalView_NilConfigDefaults(t *testing.T) {
	view := BuildSettingsModalView("C99999", nil, "ja")
	blocks := view["blocks"].([]map[string]interface{})

	getElement := func(blockID string) map[string]interface{} {
		for _, b := range blocks {
			if id, ok := b["block_id"].(string); ok && id == blockID {
				if el, ok := b["element"].(map[string]interface{}); ok {
					return el
				}
			}
		}
		return nil
	}

	// Reasonable defaults when no config exists
	if v, _ := getElement("label_name")["initial_value"].(string); v != "needs-review" {
		t.Errorf("default label_name = %q, want needs-review", v)
	}
	if v, _ := getElement("business_hours_start")["initial_value"].(string); v != "09:00" {
		t.Errorf("default business_hours_start = %q, want 09:00", v)
	}
	if v, _ := getElement("business_hours_end")["initial_value"].(string); v != "18:00" {
		t.Errorf("default business_hours_end = %q, want 18:00", v)
	}
	if v, _ := getElement("timezone")["initial_value"].(string); v != "Asia/Tokyo" {
		t.Errorf("default timezone = %q, want Asia/Tokyo", v)
	}
	if v, _ := getElement("required_approvals")["initial_value"].(string); v != "1" {
		t.Errorf("default required_approvals = %q, want 1", v)
	}
	if v, _ := getElement("reviewer_reminder_interval")["initial_value"].(string); v != "30" {
		t.Errorf("default reviewer_reminder_interval = %q, want 30", v)
	}
}

func TestParseSettingsModalSubmission(t *testing.T) {
	// view.state.values shape from Slack view_submission payload
	stateValues := map[string]map[string]ViewStateValue{
		"label_name": {
			"label_name": {Type: "plain_text_input", Value: "my-label"},
		},
		"default_mention_id": {
			"default_mention_id": {Type: "plain_text_input", Value: "U99999"},
		},
		"reviewer_list": {
			"reviewer_list": {Type: "plain_text_input", Value: "U1,U2,U3"},
		},
		"repository_list": {
			"repository_list": {Type: "plain_text_input", Value: "owner/repo1, owner/repo2"},
		},
		"reviewer_reminder_interval": {
			"reviewer_reminder_interval": {Type: "plain_text_input", Value: "45"},
		},
		"business_hours_start": {
			"business_hours_start": {Type: "plain_text_input", Value: "09:30"},
		},
		"business_hours_end": {
			"business_hours_end": {Type: "plain_text_input", Value: "18:30"},
		},
		"timezone": {
			"timezone": {Type: "plain_text_input", Value: "UTC"},
		},
		"required_approvals": {
			"required_approvals": {Type: "plain_text_input", Value: "2"},
		},
		"language": {
			"language": {Type: "static_select", SelectedOption: &ViewSelectedOption{Value: "en"}},
		},
		"is_active": {
			"is_active": {Type: "static_select", SelectedOption: &ViewSelectedOption{Value: "false"}},
		},
	}

	got, err := ParseSettingsModalSubmission(stateValues)
	if err != nil {
		t.Fatalf("ParseSettingsModalSubmission returned error: %v", err)
	}

	if got.LabelName != "my-label" {
		t.Errorf("LabelName = %q", got.LabelName)
	}
	if got.DefaultMentionID != "U99999" {
		t.Errorf("DefaultMentionID = %q", got.DefaultMentionID)
	}
	if got.ReviewerList != "U1,U2,U3" {
		t.Errorf("ReviewerList = %q", got.ReviewerList)
	}
	if got.RepositoryList != "owner/repo1,owner/repo2" {
		t.Errorf("RepositoryList = %q, want trimmed CSV", got.RepositoryList)
	}
	if got.ReviewerReminderInterval != 45 {
		t.Errorf("ReviewerReminderInterval = %d", got.ReviewerReminderInterval)
	}
	if got.BusinessHoursStart != "09:30" {
		t.Errorf("BusinessHoursStart = %q", got.BusinessHoursStart)
	}
	if got.BusinessHoursEnd != "18:30" {
		t.Errorf("BusinessHoursEnd = %q", got.BusinessHoursEnd)
	}
	if got.Timezone != "UTC" {
		t.Errorf("Timezone = %q", got.Timezone)
	}
	if got.RequiredApprovals != 2 {
		t.Errorf("RequiredApprovals = %d", got.RequiredApprovals)
	}
	if got.Language != "en" {
		t.Errorf("Language = %q", got.Language)
	}
	if got.IsActive != false {
		t.Errorf("IsActive = %v, want false", got.IsActive)
	}
}

func TestParseSettingsModalSubmission_ValidationErrors(t *testing.T) {
	mk := func(label, mention string, interval, approvals string, bhStart, bhEnd, tz, lang, active string) map[string]map[string]ViewStateValue {
		return map[string]map[string]ViewStateValue{
			"label_name":                 {"label_name": {Value: label}},
			"default_mention_id":         {"default_mention_id": {Value: mention}},
			"reviewer_list":              {"reviewer_list": {Value: ""}},
			"repository_list":            {"repository_list": {Value: ""}},
			"reviewer_reminder_interval": {"reviewer_reminder_interval": {Value: interval}},
			"business_hours_start":       {"business_hours_start": {Value: bhStart}},
			"business_hours_end":         {"business_hours_end": {Value: bhEnd}},
			"timezone":                   {"timezone": {Value: tz}},
			"required_approvals":         {"required_approvals": {Value: approvals}},
			"language":                   {"language": {SelectedOption: &ViewSelectedOption{Value: lang}}},
			"is_active":                  {"is_active": {SelectedOption: &ViewSelectedOption{Value: active}}},
		}
	}

	tests := []struct {
		name      string
		values    map[string]map[string]ViewStateValue
		wantField string
	}{
		{
			name:      "empty label",
			values:    mk("", "U1", "30", "1", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "label_name",
		},
		{
			name:      "bad reminder interval",
			values:    mk("L", "U1", "abc", "1", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "reviewer_reminder_interval",
		},
		{
			name:      "bad approvals (zero)",
			values:    mk("L", "U1", "30", "0", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "required_approvals",
		},
		{
			name:      "bad approvals (too many)",
			values:    mk("L", "U1", "30", "11", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "required_approvals",
		},
		{
			name:      "bad time format start",
			values:    mk("L", "U1", "30", "1", "9am", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "business_hours_start",
		},
		{
			name:      "bad time format end",
			values:    mk("L", "U1", "30", "1", "09:00", "25:00", "Asia/Tokyo", "ja", "true"),
			wantField: "business_hours_end",
		},
		{
			name:      "bad timezone",
			values:    mk("L", "U1", "30", "1", "09:00", "18:00", "NotAZone", "ja", "true"),
			wantField: "timezone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSettingsModalSubmission(tt.values)
			if err == nil {
				t.Fatalf("expected error")
			}
			ve, ok := err.(*ModalValidationError)
			if !ok {
				t.Fatalf("expected *ModalValidationError, got %T: %v", err, err)
			}
			if _, has := ve.Errors[tt.wantField]; !has {
				t.Errorf("expected validation error on %q, got %+v", tt.wantField, ve.Errors)
			}
		})
	}
}
