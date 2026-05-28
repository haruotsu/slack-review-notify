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
//
//   - Value           plain_text_input / number_input
//   - SelectedOption  static_select / radio_buttons
//   - SelectedOptions checkboxes / multi_static_select
//   - SelectedUser    users_select       (resolved user ID, e.g. "U123")
//   - SelectedUsers   multi_users_select (slice of user IDs)
//   - SelectedDate    datepicker         (YYYY-MM-DD)
type ViewStateValue struct {
	Type            string               `json:"type"`
	Value           string               `json:"value"`
	SelectedOption  *ViewSelectedOption  `json:"selected_option,omitempty"`
	SelectedOptions []ViewSelectedOption `json:"selected_options,omitempty"`
	SelectedUser    string               `json:"selected_user,omitempty"`
	SelectedUsers   []string             `json:"selected_users,omitempty"`
	SelectedDate    string               `json:"selected_date,omitempty"`
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

	mentionUser := ""
	mentionText := ""
	pickerReviewerIDs := []string{}
	freeTextReviewerEntries := []string{}
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
		// DefaultMentionID is a single column with mixed shapes in real configs
		// (U…, S…, "@team", decorative phrases). Route to the picker iff it
		// parses as a user ID; everything else (subteam IDs, free text) is
		// shown raw in the adjacent free-text field so the user can edit it
		// without data loss.
		if looksLikeUserID(cfg.DefaultMentionID) {
			mentionUser = cfg.DefaultMentionID
		} else {
			mentionText = cfg.DefaultMentionID
		}
		// Same split for the reviewer list: CSV entries that look like user IDs
		// go into the multi_users_select; bare names / decorative entries land
		// in the free-text CSV input so they round-trip cleanly.
		for _, r := range strings.Split(cfg.ReviewerList, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if looksLikeUserID(r) {
				pickerReviewerIDs = append(pickerReviewerIDs, r)
			} else {
				freeTextReviewerEntries = append(freeTextReviewerEntries, r)
			}
		}
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

	// Individual mention target → users_select. Native picker gives autocomplete
	// and avatars; the bot receives a pre-resolved U… ID in the submission
	// payload (no users:read scope needed).
	mentionUserElement := map[string]any{
		"type":      "users_select",
		"action_id": "default_mention_user",
	}
	if mentionUser != "" {
		mentionUserElement["initial_user"] = mentionUser
	}
	mentionUserBlock := map[string]any{
		"type":     "input",
		"block_id": "default_mention_user",
		"optional": true,
		"label":    plainText(t("modal.default_mention_user")),
		"hint":     plainText(t("modal.default_mention_user.hint")),
		"element":  mentionUserElement,
	}

	// Free-text mention field. Catches everything the user picker can't express:
	// subteam IDs (S…), decorative text, legacy bare names from pre-modal slash
	// command configs. The picker wins on save if both are set.
	mentionTextBlock := plainInput(
		"default_mention_text",
		t("modal.default_mention_text"),
		t("modal.default_mention_text.hint"),
		mentionText,
		true,
	)

	// Reviewer pool picker → multi_users_select. Plus a free-text CSV next to
	// it for non-U… entries (legacy data, decorative names).
	reviewerElement := map[string]any{
		"type":      "multi_users_select",
		"action_id": "reviewer_list",
	}
	if len(pickerReviewerIDs) > 0 {
		reviewerElement["initial_users"] = pickerReviewerIDs
	}
	reviewerBlock := map[string]any{
		"type":     "input",
		"block_id": "reviewer_list",
		"optional": true,
		"label":    plainText(t("modal.reviewer_list")),
		"hint":     plainText(t("modal.reviewer_list.hint")),
		"element":  reviewerElement,
	}
	reviewerTextBlock := plainInput(
		"reviewer_list_text",
		t("modal.reviewer_list_text"),
		t("modal.reviewer_list_text.hint"),
		strings.Join(freeTextReviewerEntries, ","),
		true,
	)

	blocks = append(blocks,
		mentionUserBlock,
		mentionTextBlock,
		reviewerBlock,
		reviewerTextBlock,
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
	selectedUser := func(blockID string) string {
		actions, ok := values[blockID]
		if !ok {
			return ""
		}
		if v, ok := actions[blockID]; ok {
			return v.SelectedUser
		}
		for _, v := range actions {
			return v.SelectedUser
		}
		return ""
	}
	selectedUsers := func(blockID string) []string {
		actions, ok := values[blockID]
		if !ok {
			return nil
		}
		if v, ok := actions[blockID]; ok {
			return v.SelectedUsers
		}
		for _, v := range actions {
			return v.SelectedUsers
		}
		return nil
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
		switch form.LabelName {
		case "":
			errs["new_label_name"] = "label name is required"
		case CreateNewLabelSentinel:
			// Reserved sentinel — saving this as a literal label name would
			// make the row unaddressable from the dropdown later.
			errs["new_label_name"] = "this label name is reserved"
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

	// Mention target. Two co-existing input shapes:
	//   - default_mention_user (users_select)    — native picker for individuals
	//   - default_mention_text (plain_text_input) — free-form: subteam ID, @team,
	//                                               decorative text, anything
	// The picker takes precedence; if it's empty we fall back to the text field
	// as-is. We don't try to resolve the text to an ID — existing channel
	// configs commonly use decorative text here, and buildMentionText handles
	// the routing at message-render time.
	if user := selectedUser("default_mention_user"); user != "" {
		form.DefaultMentionID = user
	} else {
		form.DefaultMentionID = strings.TrimSpace(field("default_mention_text"))
	}

	// Reviewer pool: multi_users_select for picked users + free-text CSV for
	// anything else (legacy bare names, decorative entries). The two lists are
	// concatenated, with picker IDs first, and stored as a single CSV.
	pickerReviewers := selectedUsers("reviewer_list")
	textReviewers := normalizeCSV(field("reviewer_list_text"))
	merged := make([]string, 0, len(pickerReviewers)+1)
	merged = append(merged, pickerReviewers...)
	if textReviewers != "" {
		merged = append(merged, strings.Split(textReviewers, ",")...)
	}
	form.ReviewerList = strings.Join(merged, ",")
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
