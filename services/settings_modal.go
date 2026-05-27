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

// SettingsModalMetadata is the JSON shape we stash in `view.private_metadata` when
// opening the settings modal. The label name is captured at open time and is NOT
// editable in the modal — this prevents users from accidentally overwriting another
// label's config by renaming. UserID is carried so the submission handler can post
// an ephemeral confirmation back to the submitter only.
type SettingsModalMetadata struct {
	ChannelID string `json:"c"`
	LabelName string `json:"l"`
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
type ViewStateValue struct {
	Type           string              `json:"type"`
	Value          string              `json:"value"`
	SelectedOption *ViewSelectedOption `json:"selected_option,omitempty"`
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
// LabelName is intentionally omitted: it is captured at modal-open time and carried
// in private_metadata, not as an editable field.
type SettingsForm struct {
	DefaultMentionID         string
	ReviewerList             string
	RepositoryList           string
	ReviewerReminderInterval int
	BusinessHoursStart       string
	BusinessHoursEnd         string
	Timezone                 string
	RequiredApprovals        int
	Language                 string
	IsActive                 bool
}

// BuildSettingsModalView returns the Slack Block Kit `view` payload for the settings modal.
//
// `labelName` identifies the (channelID, labelName) row being edited. It is rendered
// as a non-editable section block and round-tripped through private_metadata so the
// submission handler updates exactly that row — eliminating the "rename via modal
// silently overwrites another label" footgun. To add a new label config, use the
// slash command.
//
// `cfg` is the existing ChannelConfig for (channelID, labelName) if any; nil
// renders a blank-defaults modal for a fresh label.
//
// `userID` is carried through private_metadata so the submission handler can
// ephemeral-respond to the submitter only.
func BuildSettingsModalView(channelID, labelName, userID string, cfg *models.ChannelConfig, lang string) map[string]any {
	t := i18n.L(lang)

	if labelName == "" {
		labelName = "needs-review"
	}
	mentionID := ""
	reviewerList := ""
	repoList := ""
	reminderInterval := 30
	bhStart := "09:00"
	bhEnd := "18:00"
	tz := "Asia/Tokyo"
	requiredApprovals := 1
	cfgLang := lang
	if cfgLang == "" {
		cfgLang = "ja"
	}
	isActive := true

	if cfg != nil {
		mentionID = cfg.DefaultMentionID
		reviewerList = cfg.ReviewerList
		repoList = cfg.RepositoryList
		if cfg.ReviewerReminderInterval > 0 {
			reminderInterval = cfg.ReviewerReminderInterval
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
	staticSelect := func(blockID, label string, options []map[string]any, initialValue string) map[string]any {
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
		return map[string]any{
			"type":     "input",
			"block_id": blockID,
			"label":    plainText(label),
			"element":  element,
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
		// label_name is read-only: rendered as a section block so the user cannot
		// accidentally point this modal at another label and overwrite its config.
		{
			"type":     "section",
			"block_id": "label_name_display",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("%s *`%s`*", t("modal.label_name"), labelName),
			},
		},
		plainInput("default_mention_id", t("modal.default_mention_id"), t("modal.default_mention_id.hint"), mentionID, true),
		plainInput("reviewer_list", t("modal.reviewer_list"), t("modal.reviewer_list.hint"), reviewerList, true),
		plainInput("repository_list", t("modal.repository_list"), t("modal.repository_list.hint"), repoList, true),
		plainInput("reviewer_reminder_interval", t("modal.reviewer_reminder_interval"), t("modal.reviewer_reminder_interval.hint"), strconv.Itoa(reminderInterval), false),
		plainInput("business_hours_start", t("modal.business_hours_start"), "HH:MM", bhStart, false),
		plainInput("business_hours_end", t("modal.business_hours_end"), "HH:MM", bhEnd, false),
		plainInput("timezone", t("modal.timezone"), t("modal.timezone.hint"), tz, false),
		plainInput("required_approvals", t("modal.required_approvals"), t("modal.required_approvals.hint"), strconv.Itoa(requiredApprovals), false),
		staticSelect("language", t("modal.language"), langOptions, cfgLang),
		staticSelect("is_active", t("modal.is_active"), activeOptions, activeInitial),
	}

	return map[string]any{
		"type":        "modal",
		"callback_id": SettingsModalCallbackID,
		"private_metadata": EncodeSettingsModalMetadata(SettingsModalMetadata{
			ChannelID: channelID,
			LabelName: labelName,
			UserID:    userID,
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
		// Slack puts the action_id (== blockID for our inputs) as the inner key.
		if v, ok := actions[blockID]; ok {
			return strings.TrimSpace(v.Value)
		}
		// Fall back to first value (defensive).
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

	form := &SettingsForm{}

	form.DefaultMentionID = field("default_mention_id")
	form.ReviewerList = normalizeCSV(field("reviewer_list"))
	form.RepositoryList = normalizeCSV(field("repository_list"))

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
