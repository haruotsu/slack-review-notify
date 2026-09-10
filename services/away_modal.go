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

// AwayAllDayActionID is the action_id (and block_id) of the all-day checkbox
// inside the modal. Its input block sets dispatch_action, so every tick and
// untick reaches the server as a block_actions payload and the handler
// re-renders the modal with the time pickers removed or added.
const AwayAllDayActionID = "away_all_day"

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

// AwayModalPrefill is what the modal already holds when it is (re)built.
// Picker values stay in Slack's own formats (YYYY-MM-DD, HH:MM) so they can go
// straight back into initial_date / initial_time. AllDay decides which blocks
// are rendered: ticked hides the time pickers, unticked shows them.
type AwayModalPrefill struct {
	AllDay    bool
	UserID    string
	From      string
	FromTime  string
	Until     string
	UntilTime string
	Reason    string
	DeleteAll bool
}

// DefaultAwayModalPrefill is a freshly opened modal: nothing entered, all-day
// ticked. Whole-day leave is the common case, so the form starts without time
// pickers and only grows them when the user unticks the box.
func DefaultAwayModalPrefill() AwayModalPrefill {
	return AwayModalPrefill{AllDay: true}
}

// AwayManagementModalInputs is the BuildAwayManagementModalView parameter
// struct. ChannelID/UserID round-trip through private_metadata; Lang chooses
// the i18n labels; Prefill carries the current form state across a re-render.
type AwayManagementModalInputs struct {
	ChannelID string
	UserID    string
	Lang      string
	Prefill   AwayModalPrefill
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
// away-management modal. Inputs, in render order:
//
//   - away_user        users_select   (required)  — the leave subject
//   - away_from        datepicker     (optional)  — start date, nil = now
//   - away_until       datepicker     (optional)  — end date, nil = indefinite
//   - away_all_day     checkboxes     (optional)  — ticked hides the two below
//   - away_from_time   timepicker     (optional)  — only while all-day is off
//   - away_until_time  timepicker     (optional)  — only while all-day is off
//   - away_reason      plain_text     (optional)  — free-text note
//   - away_delete_all  checkboxes     (optional)  — "wipe all records" override
//
// Every element gets its initial_* value from Prefill when one is set. Slack
// keeps input state across views.update only for blocks it can match, and an
// empty initial_date / initial_time is rejected outright, so the values are
// passed explicitly and only when non-empty.
func BuildAwayManagementModalView(in AwayManagementModalInputs) map[string]any {
	t := i18n.L(in.Lang)
	p := in.Prefill

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
	datePicker := func(actionID, initial string) map[string]any {
		element := map[string]any{"type": "datepicker", "action_id": actionID}
		if initial != "" {
			element["initial_date"] = initial
		}
		return element
	}
	timePicker := func(actionID, initial string) map[string]any {
		element := map[string]any{"type": "timepicker", "action_id": actionID}
		if initial != "" {
			element["initial_time"] = initial
		}
		return element
	}
	// A single-option checkbox. The same option map is reused for
	// initial_options because Slack requires an exact match with options.
	checkbox := func(blockID, label, hint, optionText string, checked, dispatch bool) map[string]any {
		option := map[string]any{"text": plainText(optionText), "value": "yes"}
		element := map[string]any{
			"type":      "checkboxes",
			"action_id": blockID,
			"options":   []map[string]any{option},
		}
		if checked {
			element["initial_options"] = []map[string]any{option}
		}
		block := inputBlock(blockID, label, hint, element, true)
		if dispatch {
			block["dispatch_action"] = true
		}
		return block
	}

	userElement := map[string]any{"type": "users_select", "action_id": "away_user"}
	if p.UserID != "" {
		userElement["initial_user"] = p.UserID
	}
	userBlock := inputBlock("away_user", t("modal.away.user"), t("modal.away.user.hint"), userElement, false)

	fromBlock := inputBlock("away_from", t("modal.away.from"), t("modal.away.from.hint"),
		datePicker("away_from", p.From), true)
	untilBlock := inputBlock("away_until", t("modal.away.until"), t("modal.away.until.hint"),
		datePicker("away_until", p.Until), true)

	allDayBlock := checkbox(AwayAllDayActionID, t("modal.away.all_day"), t("modal.away.all_day.hint"),
		t("modal.away.all_day.option"), p.AllDay, true)

	fromTimeBlock := inputBlock("away_from_time", t("modal.away.from_time"), t("modal.away.from_time.hint"),
		timePicker("away_from_time", p.FromTime), true)
	untilTimeBlock := inputBlock("away_until_time", t("modal.away.until_time"), t("modal.away.until_time.hint"),
		timePicker("away_until_time", p.UntilTime), true)

	reasonElement := map[string]any{
		"type":       "plain_text_input",
		"action_id":  "away_reason",
		"max_length": 500,
	}
	if p.Reason != "" {
		reasonElement["initial_value"] = p.Reason
	}
	reasonBlock := inputBlock("away_reason", t("modal.away.reason"), t("modal.away.reason.hint"), reasonElement, true)

	deleteAllBlock := checkbox("away_delete_all", t("modal.away.delete_all"), t("modal.away.delete_all.hint"),
		t("modal.away.delete_all.option"), p.DeleteAll, false)

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
		allDayBlock,
	}
	if !p.AllDay {
		blocks = append(blocks, fromTimeBlock, untilTimeBlock)
	}
	blocks = append(blocks, reasonBlock, deleteAllBlock)

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

// awayStateValue returns the element state stored under blockID. Every element
// in this modal uses its block_id as action_id, but a handcrafted payload may
// key the inner map differently, so fall back to the first entry.
func awayStateValue(values map[string]map[string]ViewStateValue, blockID string) ViewStateValue {
	actions, ok := values[blockID]
	if !ok {
		return ViewStateValue{}
	}
	if v, ok := actions[blockID]; ok {
		return v
	}
	for _, v := range actions {
		return v
	}
	return ViewStateValue{}
}

// awayStateDate reads a datepicker. Slack delivers the pick in selected_date;
// value is honored too so handcrafted test payloads still work.
func awayStateDate(values map[string]map[string]ViewStateValue, blockID string) string {
	v := awayStateValue(values, blockID)
	if v.SelectedDate != "" {
		return v.SelectedDate
	}
	return strings.TrimSpace(v.Value)
}

// awayStateChecked reports whether a single-option ("yes") checkbox is ticked.
func awayStateChecked(values map[string]map[string]ViewStateValue, blockID string) bool {
	for _, opt := range awayStateValue(values, blockID).SelectedOptions {
		if opt.Value == "yes" {
			return true
		}
	}
	return false
}

// AwayModalPrefillFromState turns the view.state.values of a block_actions
// payload into the prefill for the next render, so toggling all-day keeps
// everything else the user has already entered.
func AwayModalPrefillFromState(values map[string]map[string]ViewStateValue) AwayModalPrefill {
	return AwayModalPrefill{
		AllDay:    awayStateChecked(values, AwayAllDayActionID),
		UserID:    awayStateValue(values, "away_user").SelectedUser,
		From:      awayStateDate(values, "away_from"),
		FromTime:  awayStateValue(values, "away_from_time").SelectedTime,
		Until:     awayStateDate(values, "away_until"),
		UntilTime: awayStateValue(values, "away_until_time").SelectedTime,
		Reason:    strings.TrimSpace(awayStateValue(values, "away_reason").Value),
		DeleteAll: awayStateChecked(values, "away_delete_all"),
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

	form := &AwayForm{}

	form.SlackUserID = awayStateValue(values, "away_user").SelectedUser
	if form.SlackUserID == "" {
		errs["away_user"] = t("modal.away.error.user_required")
	}

	form.DeleteAll = awayStateChecked(values, "away_delete_all")

	// While the all-day box is ticked the view carries no time pickers, but a
	// submit can race the re-render and arrive with both the tick and a time.
	// The tick wins: reading the time here would turn a whole day into a slot
	// the user has visibly opted out of.
	allDay := awayStateChecked(values, AwayAllDayActionID)

	// Dates are interpreted only in the set path; the delete-all branch
	// ignores them so users can wipe records without picking dates.
	//
	// endOfDay=true (used for `until`) anchors the timestamp at 23:59:59 +loc
	// so the leave covers the entire selected day, matching the slash-command
	// behavior. Without this, an `until` of 2030-04-05 would expire at midnight
	// the same day instead of at the end of it.
	parseDate := func(blockID, timeBlockID string, endOfDay bool) *time.Time {
		raw := awayStateDate(values, blockID)
		timeVal := ""
		if !allDay {
			timeVal = awayStateValue(values, timeBlockID).SelectedTime
		}

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
	form.Reason = strings.TrimSpace(awayStateValue(values, "away_reason").Value)

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
