package services

import (
	"encoding/json"
	"slack-review-notify/i18n"
	"strings"
	"time"
)

// AwayManagementModalCallbackID identifies the away-management modal in
// view_submission payloads. Kept distinct from the settings modal so the
// dispatch in HandleSlackAction routes to the right handler.
const AwayManagementModalCallbackID = "away_management_modal"

// OpenAwayManagementActionID is the action_id of the help-command button that
// opens this modal. Slack delivers it as a block_actions payload and the
// handler responds with views.open.
const OpenAwayManagementActionID = "open_away_management"

// AwayModalMetadata is what we stash in `view.private_metadata` so the
// submission handler knows where to post its confirmation back.
type AwayModalMetadata struct {
	ChannelID string `json:"c"`
	UserID    string `json:"u"`
}

// EncodeAwayModalMetadata serializes the struct for view.private_metadata.
func EncodeAwayModalMetadata(m AwayModalMetadata) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// DecodeAwayModalMetadata parses view.private_metadata. Empty input yields
// a zero value with no error so callers can treat it uniformly.
func DecodeAwayModalMetadata(s string) (AwayModalMetadata, error) {
	var m AwayModalMetadata
	if s == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return m, err
	}
	return m, nil
}

// AwayManagementModalInputs is the BuildAwayManagementModalView parameter
// struct. ChannelID/UserID round-trip through private_metadata; Lang chooses
// the i18n labels.
type AwayManagementModalInputs struct {
	ChannelID string
	UserID    string
	Lang      string
}

// AwayForm is the parsed view_submission. SlackUserID is always set when the
// returned error is nil. DeleteAll, when true, means "wipe every leave row
// for this user" — the date fields are ignored in that branch.
type AwayForm struct {
	SlackUserID string
	AwayFrom    *time.Time
	AwayUntil   *time.Time
	Reason      string
	DeleteAll   bool
}

// BuildAwayManagementModalView returns the Slack Block Kit view for the
// away-management modal. Five inputs:
//
//   - away_user        users_select   (required)  — the leave subject
//   - away_from        datepicker     (optional)  — start date, nil = now
//   - away_until       datepicker     (optional)  — end date, nil = indefinite
//   - away_reason      plain_text     (optional)  — free-text note
//   - away_delete_all  checkboxes     (optional)  — "wipe all records" override
//
// Dates are stored in DB as time.Time; date-only datepicker values are
// interpreted as midnight UTC, matching the convention of the slash command.
func BuildAwayManagementModalView(in AwayManagementModalInputs) map[string]any {
	t := i18n.L(in.Lang)

	plainText := func(s string) map[string]any {
		return map[string]any{"type": "plain_text", "text": s}
	}
	inputBlock := func(blockID, label, hint string, element map[string]any, optional bool) map[string]any {
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

	userBlock := inputBlock(
		"away_user",
		t("modal.away.user"),
		t("modal.away.user.hint"),
		map[string]any{
			"type":      "users_select",
			"action_id": "away_user",
		},
		false,
	)

	fromBlock := inputBlock(
		"away_from",
		t("modal.away.from"),
		t("modal.away.from.hint"),
		map[string]any{
			"type":      "datepicker",
			"action_id": "away_from",
		},
		true,
	)

	untilBlock := inputBlock(
		"away_until",
		t("modal.away.until"),
		t("modal.away.until.hint"),
		map[string]any{
			"type":      "datepicker",
			"action_id": "away_until",
		},
		true,
	)

	reasonBlock := inputBlock(
		"away_reason",
		t("modal.away.reason"),
		t("modal.away.reason.hint"),
		map[string]any{
			"type":      "plain_text_input",
			"action_id": "away_reason",
		},
		true,
	)

	deleteAllBlock := inputBlock(
		"away_delete_all",
		t("modal.away.delete_all"),
		t("modal.away.delete_all.hint"),
		map[string]any{
			"type":      "checkboxes",
			"action_id": "away_delete_all",
			"options": []map[string]any{
				{
					"text":  plainText(t("modal.away.delete_all.option")),
					"value": "yes",
				},
			},
		},
		true,
	)

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": t("modal.away.header"),
			},
		},
		userBlock,
		fromBlock,
		untilBlock,
		reasonBlock,
		deleteAllBlock,
	}

	return map[string]any{
		"type":        "modal",
		"callback_id": AwayManagementModalCallbackID,
		"private_metadata": EncodeAwayModalMetadata(AwayModalMetadata{
			ChannelID: in.ChannelID,
			UserID:    in.UserID,
		}),
		"title":  plainText(t("modal.away.title")),
		"submit": plainText(t("modal.away.submit")),
		"close":  plainText(t("modal.away.close")),
		"blocks": blocks,
	}
}

// ParseAwayModalSubmission converts the view.state.values map into an
// AwayForm. Validation errors (missing user, malformed date, from > until)
// are returned as *ModalValidationError with per-field keys so Slack can
// highlight the offending input.
func ParseAwayModalSubmission(values map[string]map[string]ViewStateValue) (*AwayForm, error) {
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
	checkboxChecked := func(blockID, optionValue string) bool {
		actions, ok := values[blockID]
		if !ok {
			return false
		}
		v, ok := actions[blockID]
		if !ok {
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

	form := &AwayForm{}

	form.SlackUserID = selectedUser("away_user")
	if form.SlackUserID == "" {
		errs["away_user"] = "select a user"
	}

	form.DeleteAll = checkboxChecked("away_delete_all", "yes")

	// Dates are interpreted only in the set path; the delete-all branch
	// ignores them so users can wipe records without picking dates.
	// Slack's datepicker delivers the value in `selected_date`, but we also
	// fall back to `value` so handcrafted test payloads still work.
	parseDate := func(blockID string) *time.Time {
		actions, ok := values[blockID]
		if !ok {
			return nil
		}
		var raw string
		if v, ok := actions[blockID]; ok {
			if v.SelectedDate != "" {
				raw = v.SelectedDate
			} else {
				raw = strings.TrimSpace(v.Value)
			}
		}
		if raw == "" {
			return nil
		}
		ts, err := time.Parse("2006-01-02", raw)
		if err != nil {
			errs[blockID] = "must be YYYY-MM-DD"
			return nil
		}
		return &ts
	}

	form.AwayFrom = parseDate("away_from")
	form.AwayUntil = parseDate("away_until")
	form.Reason = field("away_reason")

	if form.AwayFrom != nil && form.AwayUntil != nil && !form.AwayFrom.Before(*form.AwayUntil) {
		errs["away_until"] = "must be after the start date"
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}
