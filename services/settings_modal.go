package services

import (
	"crypto/sha1"
	"encoding/hex"
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

// SettingsFieldBlockID returns the block_id a settings field is rendered under
// while labelName (or CreateNewLabelSentinel) is selected in the dropdown.
//
// The id is not the bare field name on purpose. Slack carries an input's value
// over a views.update whenever its block_id and action_id are unchanged, and
// ignores the new initial_value. With fixed ids, switching the dropdown from one
// label to another would keep the first label's values on screen, and saving
// would write them onto the label the user had just picked. The label is hashed
// rather than embedded because block_id is capped at 255 characters and a label
// name is not.
func SettingsFieldBlockID(field, labelName string) string {
	sum := sha1.Sum([]byte(labelName))
	return field + ":" + hex.EncodeToString(sum[:8])
}

// RenameLabelBlockID returns the block_id of the rename field for one label.
func RenameLabelBlockID(labelName string) string {
	return SettingsFieldBlockID(renameLabelField, labelName)
}

const (
	renameLabelField  = "rename_label"
	newLabelNameField = "new_label_name"
)

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
	SelectedTime    string               `json:"selected_time,omitempty"`
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
	RenameTo                 string // new name for an existing label; empty keeps the current name
	RenameBlockID            string // block the rename field was read from, for reporting errors on it
	DeleteBlockID            string // block the delete checkbox was read from, for reporting errors on it
	CreateNew                bool   // true when the user picked the create-new sentinel
	DeleteConfig             bool   // true when the delete checkbox was checked
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
	NotifyOnHolidays         *bool // nil when the submitted modal predates the field; keep the stored value
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
	notifyOnHolidays := false

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
		notifyOnHolidays = cfg.NotifyOnHolidays
	}

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

	// label_select must keep its id: it is the field the user just changed, and
	// its new value is what this render is for. new_label_name is only rendered
	// in create-new mode, so there is no other label's value it could carry.
	blockIDFor := func(field string) string {
		if field == LabelSelectActionID || field == newLabelNameField {
			return field
		}
		return SettingsFieldBlockID(field, initialLabel)
	}

	plainText := func(s string) map[string]any {
		return map[string]any{"type": "plain_text", "text": s}
	}
	plainInput := func(field, label, hint, initial string, optional bool) map[string]any {
		element := map[string]any{
			"type":      "plain_text_input",
			"action_id": field,
		}
		if initial != "" {
			element["initial_value"] = initial
		}
		block := map[string]any{
			"type":     "input",
			"block_id": blockIDFor(field),
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
	staticSelect := func(field, label string, options []map[string]any, initialValue string, dispatchAction bool) map[string]any {
		element := map[string]any{
			"type":      "static_select",
			"action_id": field,
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
			"block_id": blockIDFor(field),
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
	notifyOnHolidaysOptions := []map[string]any{
		option(t("modal.notify_on_holidays.true"), "true"),
		option(t("modal.notify_on_holidays.false"), "false"),
	}
	notifyOnHolidaysInitial := strconv.FormatBool(notifyOnHolidays)

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": t("modal.header"),
			},
		},
		staticSelect(LabelSelectActionID, t("modal.label_select"), labelOptions, initialLabel, true),
	}

	if creatingNew {
		// new_label_name is only rendered in create-new mode so editing an
		// existing label never silently triggers a rename.
		blocks = append(blocks, plainInput(
			newLabelNameField,
			t("modal.new_label_name"),
			t("modal.new_label_name.hint"),
			"",
			false,
		))
	} else {
		// Kept apart from new_label_name: that field names a row being created,
		// this one moves an existing row, and sharing a block would let a stale
		// value from one mode act in the other after the dropdown re-renders.
		blocks = append(blocks, plainInput(
			renameLabelField,
			t("modal.rename_label"),
			t("modal.rename_label.hint"),
			"",
			true,
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
		"block_id": blockIDFor("default_mention_user"),
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
		"block_id": blockIDFor("reviewer_list"),
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
		staticSelect("notify_on_holidays", t("modal.notify_on_holidays"), notifyOnHolidaysOptions, notifyOnHolidaysInitial, false),
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
			"block_id": blockIDFor("delete_config"),
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

	// Blocks are looked up by block_id and their element by action_id: the two
	// differ for fields scoped to a label (see SettingsFieldBlockID). A
	// handcrafted payload may key the inner map differently, so fall back to the
	// block's only element.
	state := func(blockID, actionID string) ViewStateValue {
		actions := values[blockID]
		if v, ok := actions[actionID]; ok {
			return v
		}
		for _, v := range actions {
			return v
		}
		return ViewStateValue{}
	}
	field := func(blockID, actionID string) string {
		return strings.TrimSpace(state(blockID, actionID).Value)
	}
	selectField := func(blockID, actionID string) string {
		if opt := state(blockID, actionID).SelectedOption; opt != nil {
			return opt.Value
		}
		return ""
	}
	selectedUser := func(blockID, actionID string) string {
		return state(blockID, actionID).SelectedUser
	}
	selectedUsers := func(blockID, actionID string) []string {
		return state(blockID, actionID).SelectedUsers
	}
	checkboxChecked := func(blockID, actionID, optionValue string) bool {
		for _, opt := range state(blockID, actionID).SelectedOptions {
			if opt.Value == optionValue {
				return true
			}
		}
		return false
	}

	form := &SettingsForm{}

	// Label resolution: when the user picks the create-new sentinel, take the
	// name from the new_label_name text input; otherwise use the selected value.
	selected := selectField(LabelSelectActionID, LabelSelectActionID)
	if selected == CreateNewLabelSentinel {
		form.CreateNew = true
		form.LabelName = field(newLabelNameField, newLabelNameField)
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

	// Each field is read from, and its error reported on, the block rendered for
	// the label the dropdown was on at submit (see SettingsFieldBlockID).
	scope := form.LabelName
	if form.CreateNew {
		scope = CreateNewLabelSentinel
	}
	blockID := func(field string) string {
		id := SettingsFieldBlockID(field, scope)
		if _, ok := values[id]; ok {
			return id
		}
		// A modal opened before ids were label-scoped can still be submitted
		// after a deploy; rejecting it would throw away what the user typed.
		if _, ok := values[field]; ok {
			return field
		}
		return id
	}
	// at returns the block_id and action_id a field is read from.
	at := func(field string) (string, string) {
		return blockID(field), field
	}

	// The dropdown can change before the views.update that re-renders the other
	// fields reaches the client. Such a submit carries another label's blocks:
	// reading them would save the wrong values, and errors keyed by the selected
	// label's blocks would point at fields Slack is not showing.
	if selected != "" && !renderedFor(values, scope) {
		return nil, &ModalValidationError{Errors: map[string]string{
			LabelSelectActionID: "the form had not switched to this label yet; check the values and submit again",
		}}
	}

	// Delete checkbox short-circuits the rest of validation: when the user
	// asked to delete the row, the other fields don't need to be valid.
	form.DeleteBlockID = blockID("delete_config")
	form.DeleteConfig = checkboxChecked(form.DeleteBlockID, "delete_config", "yes")
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
	if user := selectedUser(at("default_mention_user")); user != "" {
		form.DefaultMentionID = user
	} else {
		form.DefaultMentionID = strings.TrimSpace(field(at("default_mention_text")))
	}

	// Reviewer pool: multi_users_select for picked users + free-text CSV for
	// anything else (legacy bare names, decorative entries). The two lists are
	// concatenated, with picker IDs first, and stored as a single CSV.
	pickerReviewers := selectedUsers(at("reviewer_list"))
	textReviewers := normalizeCSV(field(at("reviewer_list_text")))
	merged := make([]string, 0, len(pickerReviewers)+1)
	merged = append(merged, pickerReviewers...)
	if textReviewers != "" {
		merged = append(merged, strings.Split(textReviewers, ",")...)
	}
	form.ReviewerList = strings.Join(merged, ",")
	form.RepositoryList = normalizeCSV(field(at("repository_list")))

	if interval, err := strconv.Atoi(field(at("reminder_interval"))); err != nil || interval <= 0 {
		errs[blockID("reminder_interval")] = "must be a positive integer"
	} else {
		form.ReminderInterval = interval
	}

	intervalStr := field(at("reviewer_reminder_interval"))
	if interval, err := strconv.Atoi(intervalStr); err != nil || interval <= 0 {
		errs[blockID("reviewer_reminder_interval")] = "must be a positive integer"
	} else {
		form.ReviewerReminderInterval = interval
	}

	form.BusinessHoursStart = field(at("business_hours_start"))
	if !isValidHHMM(form.BusinessHoursStart) {
		errs[blockID("business_hours_start")] = "must be HH:MM"
	}
	form.BusinessHoursEnd = field(at("business_hours_end"))
	if !isValidHHMM(form.BusinessHoursEnd) {
		errs[blockID("business_hours_end")] = "must be HH:MM"
	}

	form.Timezone = field(at("timezone"))
	if _, err := time.LoadLocation(form.Timezone); err != nil {
		errs[blockID("timezone")] = "invalid IANA timezone"
	}

	approvalsStr := field(at("required_approvals"))
	if approvals, err := strconv.Atoi(approvalsStr); err != nil || approvals < 1 || approvals > 10 {
		errs[blockID("required_approvals")] = "must be 1-10"
	} else {
		form.RequiredApprovals = approvals
	}

	form.Language = selectField(at("language"))
	if form.Language != "ja" && form.Language != "en" {
		errs[blockID("language")] = "must be ja or en"
	}

	if !form.CreateNew {
		form.RenameBlockID = blockID(renameLabelField)
		form.RenameTo = field(form.RenameBlockID, renameLabelField)
		if form.RenameTo == form.LabelName {
			form.RenameTo = ""
		}
		if form.RenameTo == CreateNewLabelSentinel {
			errs[form.RenameBlockID] = i18n.TWithLang(form.Language, "modal.rename_label.reserved")
		}
	}

	// A modal opened before this field existed submits without it. Rejecting the
	// whole form or defaulting to "off" would silently undo a value set by command.
	switch notifyOnHolidays := selectField(at("notify_on_holidays")); notifyOnHolidays {
	case "true", "false":
		enabled := notifyOnHolidays == "true"
		form.NotifyOnHolidays = &enabled
	case "":
	default:
		errs[blockID("notify_on_holidays")] = "must be true or false"
	}

	activeStr := selectField(at("is_active"))
	switch activeStr {
	case "true":
		form.IsActive = true
	case "false":
		form.IsActive = false
	default:
		errs[blockID("is_active")] = "must be true or false"
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}

// renderedFor reports whether values came from a view rendered for scope (a
// label name or CreateNewLabelSentinel). A view whose block_ids are not scoped
// to any label predates SettingsFieldBlockID and is accepted as is; is_active is
// the marker for it because it is rendered in every mode.
func renderedFor(values map[string]map[string]ViewStateValue, scope string) bool {
	suffix := SettingsFieldBlockID("", scope)
	for id := range values {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	_, unscoped := values["is_active"]
	return unscoped
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
