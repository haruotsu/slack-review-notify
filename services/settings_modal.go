package services

import (
	"encoding/json"
	"fmt"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"strconv"
	"strings"
	"time"
)

// SettingsModalCallbackID is the callback_id used by the settings modal view.
const SettingsModalCallbackID = "settings_modal"

// LabelSelectActionID is the action_id of the label dropdown. block_actions
// payloads with this action_id trigger a views.update so the modal can re-render
// with the newly-selected label's values pre-filled.
const LabelSelectActionID = "label_select"

// CreateNewLabelSentinel is the special dropdown value indicating the user
// wants to create a new label configuration. When this value is selected,
// BuildSettingsModalView renders a new_label_name input below the dropdown.
const CreateNewLabelSentinel = "__create_new__"

// SettingsModalMetadata is what we stash in `view.private_metadata`. Channel ID
// and user ID are stable across re-renders of the modal; the currently-selected
// label is NOT here because it is the form's source of truth (a dropdown).
type SettingsModalMetadata struct {
	ChannelID string `json:"c"`
	UserID    string `json:"u"`
}

// EncodeSettingsModalMetadata serializes metadata for view.private_metadata.
func EncodeSettingsModalMetadata(m SettingsModalMetadata) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// DecodeSettingsModalMetadata parses view.private_metadata. Empty input returns
// a zero-value struct and no error so callers can treat it uniformly.
func DecodeSettingsModalMetadata(s string) (SettingsModalMetadata, error) {
	var m SettingsModalMetadata
	if s == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return m, err
	}
	return m, nil
}

// ViewSelectedOption mirrors the option payload Slack sends back in view_submission.
type ViewSelectedOption struct {
	Value string `json:"value"`
	Text  struct {
		Text string `json:"text"`
	} `json:"text"`
}

// ViewStateValue mirrors a single field's state in a Slack view_submission payload.
// SelectedOptions (plural) is populated for checkboxes / multi_static_select; the
// singular SelectedOption is populated for static_select / radio_buttons.
type ViewStateValue struct {
	Type            string               `json:"type"`
	Value           string               `json:"value"`
	SelectedOption  *ViewSelectedOption  `json:"selected_option,omitempty"`
	SelectedOptions []ViewSelectedOption `json:"selected_options,omitempty"`
}

// ModalValidationError carries per-field error messages for views.update / response_action: errors.
type ModalValidationError struct {
	Errors map[string]string
}

func (e *ModalValidationError) Error() string {
	parts := make([]string, 0, len(e.Errors))
	for k, v := range e.Errors {
		parts = append(parts, fmt.Sprintf("%s: %s", k, v))
	}
	return strings.Join(parts, "; ")
}

// SettingsForm is the parsed view_submission payload, ready to upsert into ChannelConfig.
type SettingsForm struct {
	LabelName                string
	CreateNew                bool // true when the user picked the create-new sentinel
	DeleteConfig             bool // true when the delete checkbox was checked
	DefaultMentionID         string
	ReviewerList             string
	RepositoryList           string
	ReminderInterval         int
	ReviewerReminderInterval int
	BusinessHoursStart       string
	BusinessHoursEnd         string
	Timezone                 string
	RequiredApprovals        int
	Language                 string
	IsActive                 bool
}

// SettingsModalInputs is the parameter struct for BuildSettingsModalView. Using
// a struct keeps the call sites readable as we add more options (selected label,
// known labels, language…) without ballooning the positional argument list.
type SettingsModalInputs struct {
	ChannelID     string
	UserID        string
	SelectedLabel string                  // currently-displayed label, or CreateNewLabelSentinel
	Configs       []*models.ChannelConfig // all known label configs in this channel
	Lang          string
}

// BuildSettingsModalView returns the Slack Block Kit `view` payload for the
// settings modal. The first block is a label dropdown listing every existing
// label configuration in the channel plus a "+ new label" sentinel option;
// changing the dropdown triggers a views.update via dispatch_action so the rest
// of the form re-renders pre-filled for the newly selected label.
func BuildSettingsModalView(in SettingsModalInputs) map[string]any {
	t := i18n.L(in.Lang)

	creatingNew := in.SelectedLabel == CreateNewLabelSentinel
	var cfg *models.ChannelConfig
	if !creatingNew {
		for _, c := range in.Configs {
			if c != nil && c.LabelName == in.SelectedLabel {
				cfg = c
				break
			}
		}
	}

	mentionID := ""
	reviewerList := ""
	repoList := ""
	reminderInterval := 30
	reviewerReminderInterval := 30
	bhStart := "09:00"
	bhEnd := "18:00"
	tz := "Asia/Tokyo"
	requiredApprovals := 1
	cfgLang := in.Lang
	if cfgLang == "" {
		cfgLang = "ja"
	}
	isActive := true

	if cfg != nil {
		mentionID = cfg.DefaultMentionID
		reviewerList = cfg.ReviewerList
		repoList = cfg.RepositoryList
		if cfg.ReminderInterval > 0 {
			reminderInterval = cfg.ReminderInterval
		}
		if cfg.ReviewerReminderInterval > 0 {
			reviewerReminderInterval = cfg.ReviewerReminderInterval
		}
		if cfg.BusinessHoursStart != "" {
			bhStart = cfg.BusinessHoursStart
		}
		if cfg.BusinessHoursEnd != "" {
			bhEnd = cfg.BusinessHoursEnd
		}
		if cfg.Timezone != "" {
			tz = cfg.Timezone
		}
		if cfg.RequiredApprovals > 0 {
			requiredApprovals = cfg.RequiredApprovals
		}
		if cfg.Language != "" {
			cfgLang = cfg.Language
		}
		isActive = cfg.IsActive
	}

	plainText := func(s string) map[string]any {
		return map[string]any{"type": "plain_text", "text": s}
	}
	plainInput := func(blockID, label, hint, initial string, optional bool) map[string]any {
		element := map[string]any{
			"type":      "plain_text_input",
			"action_id": blockID,
		}
		if initial != "" {
			element["initial_value"] = initial
		}
		block := map[string]any{
			"type":     "input",
			"block_id": blockID,
			"label":    plainText(label),
			"element":  element,
		}
		if hint != "" {
			block["hint"] = plainText(hint)
		}
		if optional {
			block["optional"] = true
		}
		return block
	}
	option := func(text, value string) map[string]any {
		return map[string]any{
			"text":  plainText(text),
			"value": value,
		}
	}
	staticSelect := func(blockID, label string, options []map[string]any, initialValue string, dispatchAction bool) map[string]any {
		element := map[string]any{
			"type":      "static_select",
			"action_id": blockID,
			"options":   options,
		}
		for _, opt := range options {
			if v, _ := opt["value"].(string); v == initialValue {
				element["initial_option"] = opt
				break
			}
		}
		block := map[string]any{
			"type":     "input",
			"block_id": blockID,
			"label":    plainText(label),
			"element":  element,
		}
		if dispatchAction {
			// dispatch_action tells Slack to fire a block_actions callback
			// when the value changes, which lets us rebuild the modal with
			// fresh defaults for the newly-chosen label.
			block["dispatch_action"] = true
		}
		return block
	}

	// Label dropdown: existing labels + the create-new sentinel.
	labelOptions := make([]map[string]any, 0, len(in.Configs)+1)
	seen := map[string]bool{}
	for _, c := range in.Configs {
		if c == nil || c.LabelName == "" || seen[c.LabelName] {
			continue
		}
		seen[c.LabelName] = true
		labelOptions = append(labelOptions, option(c.LabelName, c.LabelName))
	}
	labelOptions = append(labelOptions, option(t("modal.label_select.create_new"), CreateNewLabelSentinel))

	// If the requested SelectedLabel isn't in the channel's configs (e.g. the
	// help button passes "needs-review" before any config exists), force the
	// dropdown to start on the create-new option so the user isn't presented
	// with an unmatched initial selection.
	initialLabel := in.SelectedLabel
	if !creatingNew {
		hasMatch := false
		for _, c := range in.Configs {
			if c != nil && c.LabelName == initialLabel {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			initialLabel = CreateNewLabelSentinel
			creatingNew = true
		}
	}

	langOptions := []map[string]any{
		option(t("modal.lang.ja"), "ja"),
		option(t("modal.lang.en"), "en"),
	}
	activeOptions := []map[string]any{
		option(t("modal.active.true"), "true"),
		option(t("modal.active.false"), "false"),
	}
	activeInitial := "true"
	if !isActive {
		activeInitial = "false"
	}

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": t("modal.header"),
			},
		},
		staticSelect("label_select", t("modal.label_select"), labelOptions, initialLabel, true),
	}

	if creatingNew {
		// new_label_name is only rendered in create-new mode so editing an
		// existing label never silently triggers a rename.
		blocks = append(blocks, plainInput(
			"new_label_name",
			t("modal.new_label_name"),
			t("modal.new_label_name.hint"),
			"",
			false,
		))
	}

	blocks = append(blocks,
		plainInput("default_mention_id", t("modal.default_mention_id"), t("modal.default_mention_id.hint"), mentionID, true),
		plainInput("reviewer_list", t("modal.reviewer_list"), t("modal.reviewer_list.hint"), reviewerList, true),
		plainInput("repository_list", t("modal.repository_list"), t("modal.repository_list.hint"), repoList, true),
		plainInput("reminder_interval", t("modal.reminder_interval"), t("modal.reminder_interval.hint"), strconv.Itoa(reminderInterval), false),
		plainInput("reviewer_reminder_interval", t("modal.reviewer_reminder_interval"), t("modal.reviewer_reminder_interval.hint"), strconv.Itoa(reviewerReminderInterval), false),
		plainInput("business_hours_start", t("modal.business_hours_start"), "HH:MM", bhStart, false),
		plainInput("business_hours_end", t("modal.business_hours_end"), "HH:MM", bhEnd, false),
		plainInput("timezone", t("modal.timezone"), t("modal.timezone.hint"), tz, false),
		plainInput("required_approvals", t("modal.required_approvals"), t("modal.required_approvals.hint"), strconv.Itoa(requiredApprovals), false),
		staticSelect("language", t("modal.language"), langOptions, cfgLang, false),
		staticSelect("is_active", t("modal.is_active"), activeOptions, activeInitial, false),
	)

	if !creatingNew {
		// "Delete this configuration" checkbox. Only meaningful when editing
		// an existing label — in create-new mode there's nothing to delete.
		deleteOption := option(t("modal.delete_config.option"), "yes")
		blocks = append(blocks, map[string]any{
			"type":     "input",
			"block_id": "delete_config",
			"optional": true,
			"label":    plainText(t("modal.delete_config")),
			"element": map[string]any{
				"type":      "checkboxes",
				"action_id": "delete_config",
				"options":   []map[string]any{deleteOption},
			},
		})
	}

	return map[string]any{
		"type":        "modal",
		"callback_id": SettingsModalCallbackID,
		"private_metadata": EncodeSettingsModalMetadata(SettingsModalMetadata{
			ChannelID: in.ChannelID,
			UserID:    in.UserID,
		}),
		"title":  plainText(t("modal.title")),
		"submit": plainText(t("modal.submit")),
		"close":  plainText(t("modal.close")),
		"blocks": blocks,
	}
}

// ParseSettingsModalSubmission converts the view.state.values map from a Slack
// view_submission payload into a SettingsForm. Returns *ModalValidationError on bad input.
func ParseSettingsModalSubmission(values map[string]map[string]ViewStateValue) (*SettingsForm, error) {
	errs := map[string]string{}

	field := func(blockID string) string {
		actions, ok := values[blockID]
		if !ok {
			return ""
		}
		if v, ok := actions[blockID]; ok {
			return strings.TrimSpace(v.Value)
		}
		for _, v := range actions {
			return strings.TrimSpace(v.Value)
		}
		return ""
	}
	selectField := func(blockID string) string {
		actions, ok := values[blockID]
		if !ok {
			return ""
		}
		if v, ok := actions[blockID]; ok && v.SelectedOption != nil {
			return v.SelectedOption.Value
		}
		for _, v := range actions {
			if v.SelectedOption != nil {
				return v.SelectedOption.Value
			}
		}
		return ""
	}
	checkboxChecked := func(blockID, optionValue string) bool {
		actions, ok := values[blockID]
		if !ok {
			return false
		}
		v, ok := actions[blockID]
		if !ok {
			// Fall back to the first value in the block.
			for _, x := range actions {
				v = x
				break
			}
		}
		for _, opt := range v.SelectedOptions {
			if opt.Value == optionValue {
				return true
			}
		}
		return false
	}

	form := &SettingsForm{}

	// Label resolution: when the user picks the create-new sentinel, take the
	// name from the new_label_name text input; otherwise use the selected value.
	selected := selectField("label_select")
	if selected == CreateNewLabelSentinel {
		form.CreateNew = true
		form.LabelName = field("new_label_name")
		if form.LabelName == "" {
			errs["new_label_name"] = "label name is required"
		}
	} else if selected != "" {
		form.LabelName = selected
	}

	// Delete checkbox short-circuits the rest of validation: when the user
	// asked to delete the row, the other fields don't need to be valid.
	form.DeleteConfig = checkboxChecked("delete_config", "yes")
	if form.DeleteConfig && !form.CreateNew {
		// Still need the label name to know what to delete; selectField above
		// already set it. No further validation needed.
		return form, nil
	}

	// DefaultMentionID accepts a U-ID, S-ID, @username, or @subteam-handle.
	rawMention := strings.TrimPrefix(strings.TrimSpace(field("default_mention_id")), "@")
	if rawMention == "" {
		form.DefaultMentionID = ""
	} else {
		resolved, err := ResolveMentionTarget(rawMention)
		if err != nil {
			if _, ok := err.(*SlackLookupNotFoundError); ok {
				errs["default_mention_id"] = fmt.Sprintf("could not find Slack user or subteam %q", rawMention)
			} else {
				errs["default_mention_id"] = "Slack lookup failed: " + err.Error()
			}
		} else {
			form.DefaultMentionID = resolved
		}
	}

	resolvedReviewers, reviewerErr := resolveReviewerCSV(field("reviewer_list"))
	if reviewerErr != "" {
		errs["reviewer_list"] = reviewerErr
	} else {
		form.ReviewerList = resolvedReviewers
	}
	form.RepositoryList = normalizeCSV(field("repository_list"))

	if interval, err := strconv.Atoi(field("reminder_interval")); err != nil || interval <= 0 {
		errs["reminder_interval"] = "must be a positive integer"
	} else {
		form.ReminderInterval = interval
	}

	intervalStr := field("reviewer_reminder_interval")
	if interval, err := strconv.Atoi(intervalStr); err != nil || interval <= 0 {
		errs["reviewer_reminder_interval"] = "must be a positive integer"
	} else {
		form.ReviewerReminderInterval = interval
	}

	form.BusinessHoursStart = field("business_hours_start")
	if !isValidHHMM(form.BusinessHoursStart) {
		errs["business_hours_start"] = "must be HH:MM"
	}
	form.BusinessHoursEnd = field("business_hours_end")
	if !isValidHHMM(form.BusinessHoursEnd) {
		errs["business_hours_end"] = "must be HH:MM"
	}

	form.Timezone = field("timezone")
	if _, err := time.LoadLocation(form.Timezone); err != nil {
		errs["timezone"] = "invalid IANA timezone"
	}

	approvalsStr := field("required_approvals")
	if approvals, err := strconv.Atoi(approvalsStr); err != nil || approvals < 1 || approvals > 10 {
		errs["required_approvals"] = "must be 1-10"
	} else {
		form.RequiredApprovals = approvals
	}

	form.Language = selectField("language")
	if form.Language != "ja" && form.Language != "en" {
		errs["language"] = "must be ja or en"
	}

	activeStr := selectField("is_active")
	switch activeStr {
	case "true":
		form.IsActive = true
	case "false":
		form.IsActive = false
	default:
		errs["is_active"] = "must be true or false"
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}

// resolveReviewerCSV resolves each CSV entry (U-ID or @username) to a U-ID via
// Slack users.list. Returns ("", "") for an empty list, the canonical CSV on
// success, or ("", message) on the first unresolved entry. Reviewers must be
// individual users, not subteams (the assignment code distributes among them).
func resolveReviewerCSV(s string) (string, string) {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "@")
		id, err := LookupSlackUserID(trimmed)
		if err != nil {
			if _, ok := err.(*SlackLookupNotFoundError); ok {
				return "", fmt.Sprintf("could not find Slack user %q", trimmed)
			}
			return "", "Slack lookup failed: " + err.Error()
		}
		out = append(out, id)
	}
	return strings.Join(out, ","), ""
}

// normalizeCSV trims whitespace around each comma-separated entry and drops empty items.
func normalizeCSV(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, ",")
}

// isValidHHMM mirrors handlers.isValidTimeFormat but lives in services to avoid a cycle.
func isValidHHMM(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return false
	}
	return true
}
