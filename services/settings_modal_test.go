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
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U777",
		SelectedLabel: "needs-review",
		Configs:       []*models.ChannelConfig{cfg},
		Lang:          "ja",
	})

	if view["type"] != "modal" {
		t.Errorf("type = %v, want modal", view["type"])
	}
	if view["callback_id"] != "settings_modal" {
		t.Errorf("callback_id = %v, want settings_modal", view["callback_id"])
	}
	// private_metadata round-trips channel ID and submitter; selected label is
	// no longer in metadata because the dropdown is the source of truth at submit.
	pm, ok := view["private_metadata"].(string)
	if !ok {
		t.Fatalf("private_metadata not a string: %T", view["private_metadata"])
	}
	meta, err := DecodeSettingsModalMetadata(pm)
	if err != nil {
		t.Fatalf("private_metadata is not valid JSON: %v (%q)", err, pm)
	}
	if meta.ChannelID != "C12345" || meta.UserID != "U777" {
		t.Errorf("metadata = %+v, want {C12345, U777}", meta)
	}
	if _, ok := view["submit"].(map[string]any); !ok {
		t.Errorf("missing submit button")
	}
	if _, ok := view["close"].(map[string]any); !ok {
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
		ReminderInterval:         15,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
		RequiredApprovals:        2,
		Language:                 "ja",
		IsActive:                 true,
	}
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U1",
		SelectedLabel: "needs-review",
		Configs:       []*models.ChannelConfig{cfg},
		Lang:          "ja",
	})

	blocks, ok := view["blocks"].([]map[string]any)
	if !ok {
		t.Fatalf("blocks is not a slice: %T", view["blocks"])
	}

	// label_select replaces the old label_name_display section: it is now a
	// static_select including the existing labels plus a "create new" option.
	// All other configuration fields remain as editable inputs.
	requiredBlockIDs := []string{
		"label_select",
		"default_mention_id",
		"reviewer_list",
		"repository_list",
		"reminder_interval",
		"reviewer_reminder_interval",
		"business_hours_start",
		"business_hours_end",
		"timezone",
		"required_approvals",
		"language",
		"is_active",
		"delete_config",
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

	// In edit-mode (existing label selected), the new_label_name input must NOT
	// appear; it is only rendered when the user picks the create-new option.
	if found["new_label_name"] {
		t.Errorf("new_label_name should not appear in edit mode")
	}
}

func TestBuildSettingsModalView_InitialValues(t *testing.T) {
	cfg := &models.ChannelConfig{
		SlackChannelID:           "C12345",
		LabelName:                "my-label",
		DefaultMentionID:         "U99999",
		ReviewerList:             "U1,U2",
		RepositoryList:           "owner/repo1",
		ReminderInterval:         20,
		ReviewerReminderInterval: 45,
		BusinessHoursStart:       "10:00",
		BusinessHoursEnd:         "19:00",
		Timezone:                 "UTC",
		RequiredApprovals:        2,
		Language:                 "en",
		IsActive:                 true,
	}
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U777",
		SelectedLabel: "my-label",
		Configs:       []*models.ChannelConfig{cfg},
		Lang:          "en",
	})
	blocks := view["blocks"].([]map[string]any)

	getElement := func(blockID string) map[string]any {
		for _, b := range blocks {
			if id, ok := b["block_id"].(string); ok && id == blockID {
				if el, ok := b["element"].(map[string]any); ok {
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
		{"default_mention_id", "initial_value", "U99999"},
		{"reviewer_list", "initial_value", "U1,U2"},
		{"repository_list", "initial_value", "owner/repo1"},
		{"reminder_interval", "initial_value", "20"},
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

	// label_select initial_option must reflect the selected label so the dropdown
	// opens preselected to the label being edited.
	selectEl := getElement("label_select")
	if selectEl == nil {
		t.Fatalf("label_select element not found")
	}
	initial, ok := selectEl["initial_option"].(map[string]any)
	if !ok {
		t.Fatalf("label_select.initial_option missing")
	}
	if initial["value"] != "my-label" {
		t.Errorf("label_select.initial_option.value = %v, want my-label", initial["value"])
	}

	// language: static_select; initial_option must have value "en"
	langEl := getElement("language")
	if langEl == nil {
		t.Fatalf("language element not found")
	}
	opt, ok := langEl["initial_option"].(map[string]any)
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
	activeOpt, ok := activeEl["initial_option"].(map[string]any)
	if !ok {
		t.Fatalf("is_active.initial_option missing")
	}
	if activeOpt["value"] != "true" {
		t.Errorf("is_active.initial_option.value = %v, want true", activeOpt["value"])
	}
}

func TestBuildSettingsModalView_CreateNewMode(t *testing.T) {
	// When SelectedLabel is the create-new sentinel, the modal must:
	// 1. include a new_label_name plain_text_input,
	// 2. show defaults (no prefill from any existing config),
	// 3. NOT include the delete_config block (nothing to delete yet).
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U777",
		SelectedLabel: CreateNewLabelSentinel,
		Configs:       []*models.ChannelConfig{{LabelName: "needs-review"}},
		Lang:          "ja",
	})
	blocks := view["blocks"].([]map[string]any)

	found := make(map[string]bool)
	for _, b := range blocks {
		if id, ok := b["block_id"].(string); ok {
			found[id] = true
		}
	}
	if !found["new_label_name"] {
		t.Errorf("new_label_name input required in create-new mode")
	}
	if found["delete_config"] {
		t.Errorf("delete_config must not appear in create-new mode")
	}

	// Defaults: 09:00 / 18:00 / Asia/Tokyo / 1 approver / 30min reviewer reminder
	getInitial := func(blockID string) string {
		for _, b := range blocks {
			if id, _ := b["block_id"].(string); id == blockID {
				if el, ok := b["element"].(map[string]any); ok {
					v, _ := el["initial_value"].(string)
					return v
				}
			}
		}
		return ""
	}
	if v := getInitial("business_hours_start"); v != "09:00" {
		t.Errorf("default business_hours_start = %q", v)
	}
	if v := getInitial("timezone"); v != "Asia/Tokyo" {
		t.Errorf("default timezone = %q", v)
	}
	if v := getInitial("reviewer_reminder_interval"); v != "30" {
		t.Errorf("default reviewer_reminder_interval = %q", v)
	}
}

func TestBuildSettingsModalView_LabelSelectOptions(t *testing.T) {
	// The dropdown must list every existing label in the channel AND the
	// create-new sentinel so the user can both edit and add labels.
	configs := []*models.ChannelConfig{
		{LabelName: "needs-review"},
		{LabelName: "urgent"},
		{LabelName: "design-review"},
	}
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U777",
		SelectedLabel: "needs-review",
		Configs:       configs,
		Lang:          "ja",
	})
	blocks := view["blocks"].([]map[string]any)

	var optionValues []string
	for _, b := range blocks {
		if id, _ := b["block_id"].(string); id == "label_select" {
			el := b["element"].(map[string]any)
			opts, _ := el["options"].([]map[string]any)
			for _, o := range opts {
				if v, ok := o["value"].(string); ok {
					optionValues = append(optionValues, v)
				}
			}
		}
	}

	want := []string{"needs-review", "urgent", "design-review", CreateNewLabelSentinel}
	for _, w := range want {
		found := false
		for _, v := range optionValues {
			if v == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("label_select missing option %q (got %v)", w, optionValues)
		}
	}
}

func TestParseSettingsModalSubmission(t *testing.T) {
	stateValues := map[string]map[string]ViewStateValue{
		"label_select": {
			"label_select": {Type: "static_select", SelectedOption: &ViewSelectedOption{Value: "needs-review"}},
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
		"reminder_interval": {
			"reminder_interval": {Type: "plain_text_input", Value: "20"},
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

	if got.LabelName != "needs-review" {
		t.Errorf("LabelName = %q, want needs-review", got.LabelName)
	}
	if got.CreateNew {
		t.Errorf("CreateNew should be false when an existing label is selected")
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
	if got.ReminderInterval != 20 {
		t.Errorf("ReminderInterval = %d, want 20", got.ReminderInterval)
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
	if got.DeleteConfig {
		t.Errorf("DeleteConfig should be false when checkbox unchecked")
	}
}

func TestParseSettingsModalSubmission_CreateNew(t *testing.T) {
	// When the dropdown is on the create-new sentinel, LabelName must come from
	// the new_label_name input and CreateNew must be true.
	values := minimalValidParseValues()
	values["label_select"] = map[string]ViewStateValue{
		"label_select": {SelectedOption: &ViewSelectedOption{Value: CreateNewLabelSentinel}},
	}
	values["new_label_name"] = map[string]ViewStateValue{
		"new_label_name": {Value: "  shiny-new  "},
	}

	got, err := ParseSettingsModalSubmission(values)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !got.CreateNew {
		t.Errorf("CreateNew should be true")
	}
	if got.LabelName != "shiny-new" {
		t.Errorf("LabelName = %q, want shiny-new", got.LabelName)
	}
}

func TestParseSettingsModalSubmission_CreateNew_EmptyNameFails(t *testing.T) {
	// Selecting create-new but leaving the label name blank must surface a
	// per-field validation error on new_label_name.
	values := minimalValidParseValues()
	values["label_select"] = map[string]ViewStateValue{
		"label_select": {SelectedOption: &ViewSelectedOption{Value: CreateNewLabelSentinel}},
	}
	values["new_label_name"] = map[string]ViewStateValue{
		"new_label_name": {Value: ""},
	}

	_, err := ParseSettingsModalSubmission(values)
	if err == nil {
		t.Fatalf("expected error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("expected *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["new_label_name"]; !has {
		t.Errorf("expected new_label_name error, got %+v", ve.Errors)
	}
}

func TestParseSettingsModalSubmission_DeleteConfigCheckbox(t *testing.T) {
	// The delete_config checkbox is a checkboxes element. When checked, parsing
	// must reflect that so the submission handler can soft-delete the row.
	values := minimalValidParseValues()
	values["delete_config"] = map[string]ViewStateValue{
		"delete_config": {Type: "checkboxes", SelectedOptions: []ViewSelectedOption{
			{Value: "yes"},
		}},
	}
	got, err := ParseSettingsModalSubmission(values)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.DeleteConfig {
		t.Errorf("DeleteConfig = false, want true")
	}
}

func TestParseSettingsModalSubmission_ValidationErrors(t *testing.T) {
	mk := func(mention, interval, approvals, bhStart, bhEnd, tz, lang, active string) map[string]map[string]ViewStateValue {
		v := minimalValidParseValues()
		v["default_mention_id"] = map[string]ViewStateValue{"default_mention_id": {Value: mention}}
		v["reviewer_reminder_interval"] = map[string]ViewStateValue{"reviewer_reminder_interval": {Value: interval}}
		v["required_approvals"] = map[string]ViewStateValue{"required_approvals": {Value: approvals}}
		v["business_hours_start"] = map[string]ViewStateValue{"business_hours_start": {Value: bhStart}}
		v["business_hours_end"] = map[string]ViewStateValue{"business_hours_end": {Value: bhEnd}}
		v["timezone"] = map[string]ViewStateValue{"timezone": {Value: tz}}
		v["language"] = map[string]ViewStateValue{"language": {SelectedOption: &ViewSelectedOption{Value: lang}}}
		v["is_active"] = map[string]ViewStateValue{"is_active": {SelectedOption: &ViewSelectedOption{Value: active}}}
		return v
	}

	tests := []struct {
		name      string
		values    map[string]map[string]ViewStateValue
		wantField string
	}{
		{
			name:      "bad reminder interval",
			values:    mk("U1", "abc", "1", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "reviewer_reminder_interval",
		},
		{
			name:      "bad approvals (zero)",
			values:    mk("U1", "30", "0", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "required_approvals",
		},
		{
			name:      "bad approvals (too many)",
			values:    mk("U1", "30", "11", "09:00", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "required_approvals",
		},
		{
			name:      "bad time format start",
			values:    mk("U1", "30", "1", "9am", "18:00", "Asia/Tokyo", "ja", "true"),
			wantField: "business_hours_start",
		},
		{
			name:      "bad time format end",
			values:    mk("U1", "30", "1", "09:00", "25:00", "Asia/Tokyo", "ja", "true"),
			wantField: "business_hours_end",
		},
		{
			name:      "bad timezone",
			values:    mk("U1", "30", "1", "09:00", "18:00", "NotAZone", "ja", "true"),
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

// labelSentinelTest pins the CreateNewLabelSentinel value so accidental
// reformatting/refactoring doesn't silently break the dropdown handler that
// matches against this exact string.
func TestCreateNewLabelSentinel_Value(t *testing.T) {
	if CreateNewLabelSentinel == "" || !strings.HasPrefix(CreateNewLabelSentinel, "__") {
		t.Errorf("CreateNewLabelSentinel = %q, want a stable __-prefixed sentinel", CreateNewLabelSentinel)
	}
}
