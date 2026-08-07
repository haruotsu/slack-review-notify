package services

import (
	"encoding/json"
	"slack-review-notify/i18n"
	"strconv"
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

	fromTimeBlock := inputBlock(
		"away_from_time",
		t("modal.away.from_time"),
		t("modal.away.from_time.hint"),
		map[string]any{
			"type":      "timepicker",
			"action_id": "away_from_time",
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

	untilTimeBlock := inputBlock(
		"away_until_time",
		t("modal.away.until_time"),
		t("modal.away.until_time.hint"),
		map[string]any{
			"type":      "timepicker",
			"action_id": "away_until_time",
		},
		true,
	)

	reasonBlock := inputBlock(
		"away_reason",
		t("modal.away.reason"),
		t("modal.away.reason.hint"),
		map[string]any{
			"type":       "plain_text_input",
			"action_id":  "away_reason",
			"max_length": 500,
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
		fromTimeBlock,
		untilBlock,
		untilTimeBlock,
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

// parseClockTime parses a Slack timepicker "HH:MM" value. ok is false for
// anything the timepicker would never send, so the caller can surface an error
// instead of quietly using a day boundary.
func parseClockTime(s string) (hour, minute int, ok bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// ParseAwayModalSubmission converts the view.state.values map into an
// AwayForm. Validation errors (missing user, malformed date, from > until)
// are returned as *ModalValidationError with per-field keys so Slack can
// highlight the offending input.
//
// loc is the timezone used to interpret datepicker values; pass the
// channel-resolved Location so the saved AwayFrom/AwayUntil line up with what
// the slash command writes (00:00:00 +tz / 23:59:59 +tz). nil falls back to
// UTC, used by unit tests that don't care about tz semantics. lang chooses
// the i18n locale for the validation messages Slack renders inside the modal.
func ParseAwayModalSubmission(values map[string]map[string]ViewStateValue, loc *time.Location, lang string) (*AwayForm, error) {
	if loc == nil {
		loc = time.UTC
	}
	t := i18n.L(lang)
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

	selectedTime := func(blockID string) string {
		actions, ok := values[blockID]
		if !ok {
			return ""
		}
		if v, ok := actions[blockID]; ok {
			return v.SelectedTime
		}
		for _, v := range actions {
			return v.SelectedTime
		}
		return ""
	}

	form := &AwayForm{}

	form.SlackUserID = selectedUser("away_user")
	if form.SlackUserID == "" {
		errs["away_user"] = t("modal.away.error.user_required")
	}

	form.DeleteAll = checkboxChecked("away_delete_all", "yes")

	// Dates are interpreted only in the set path; the delete-all branch
	// ignores them so users can wipe records without picking dates.
	// Slack's datepicker delivers the value in `selected_date`, but we also
	// fall back to `value` so handcrafted test payloads still work.
	//
	// endOfDay=true (used for `until`) anchors the timestamp at 23:59:59 +loc
	// so the leave covers the entire selected day, matching the slash-command
	// behavior. Without this, an `until` of 2030-04-05 would expire at midnight
	// the same day instead of at the end of it.
	parseDate := func(blockID, timeBlockID string, endOfDay bool) *time.Time {
		var raw string
		if actions, ok := values[blockID]; ok {
			if v, ok := actions[blockID]; ok {
				if v.SelectedDate != "" {
					raw = v.SelectedDate
				} else {
					raw = strings.TrimSpace(v.Value)
				}
			}
		}
		timeVal := selectedTime(timeBlockID)

		if raw == "" {
			// Both pickers are optional, so picking a time and leaving the date
			// blank is an ordinary slip. Returning nil here would drop the time
			// without a word, and a nil away_until means "away indefinitely" —
			// the reviewer would stay excluded until somebody noticed.
			if timeVal != "" {
				errs[timeBlockID] = t("modal.away.error.date_required_for_time")
			}
			return nil
		}
		parsed, err := time.ParseInLocation("2006-01-02", raw, loc)
		if err != nil {
			errs[blockID] = t("modal.away.error.invalid_date")
			return nil
		}

		hh, mm, ss := 0, 0, 0
		if endOfDay {
			hh, mm, ss = 23, 59, 59
		}
		if timeVal != "" {
			h, m, ok := parseClockTime(timeVal)
			if !ok {
				// Slack's timepicker always sends HH:MM, so this is a malformed
				// payload rather than a user slip. Report it anyway: falling
				// through would turn the requested slot into a whole day.
				errs[timeBlockID] = t("modal.away.error.invalid_time")
				return nil
			}
			hh, mm, ss = h, m, 0
		}
		ts := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), hh, mm, ss, 0, loc)
		return &ts
	}

	// Only the set path reads the pickers. Parsing them under delete-all would
	// let a leftover time selection raise date_required_for_time and block a
	// deletion that does not use dates at all — the doc comment above promises
	// the opposite.
	if !form.DeleteAll {
		form.AwayFrom = parseDate("away_from", "away_from_time", false)
		form.AwayUntil = parseDate("away_until", "away_until_time", true)
	}
	form.Reason = field("away_reason")

	// Reject zero-length (from == until) and reversed ranges to prevent
	// no-op records. Same-day leave with different times is legitimate.
	if form.AwayFrom != nil && form.AwayUntil != nil && !form.AwayFrom.Before(*form.AwayUntil) {
		isSameDay := form.AwayFrom.Year() == form.AwayUntil.Year() && form.AwayFrom.YearDay() == form.AwayUntil.YearDay()
		if isSameDay {
			errs["away_until_time"] = t("modal.away.error.until_time_before_from")
		} else {
			errs["away_until"] = t("modal.away.error.until_before_from")
		}
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}
