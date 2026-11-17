package services

import (
	"encoding/json"
	"strings"

	"slack-review-notify/i18n"
	"slack-review-notify/models"

	"gorm.io/gorm"
)

// ChangeReviewerModalCallbackID identifies the change-reviewer modal in
// view_submission payloads. Routed in handlers.HandleSlackAction.
const ChangeReviewerModalCallbackID = "change_reviewer_modal"

const (
	ChangeReviewerTargetBlockID = "change_reviewer_target"
	ChangeReviewerNewBlockID    = "change_reviewer_new"
)

// slackRadioButtonsMaxOptions is Block Kit's hard cap on radio_buttons options.
const slackRadioButtonsMaxOptions = 10

type ChangeReviewerModalMetadata struct {
	TaskID string `json:"t"`
}

func EncodeChangeReviewerModalMetadata(m ChangeReviewerModalMetadata) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func DecodeChangeReviewerModalMetadata(s string) (ChangeReviewerModalMetadata, error) {
	var m ChangeReviewerModalMetadata
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return m, err
	}
	return m, nil
}

// ChangeReviewerForm is the parsed submission. An empty NewReviewerID means
// the user left the successor blank and wants a random draw.
type ChangeReviewerForm struct {
	OldReviewerID string
	NewReviewerID string
}

// DrawReplacementReviewer randomly picks one successor from the channel's
// reviewer list, excluding the PR author, everyone currently assigned and
// users on leave. ok is false when nobody real is left to draw.
//
// requireReviewerList rejects the DefaultMentionID fallback even when the
// channel has no reviewer list. The one-click button keeps accepting it so its
// existing behavior is unchanged; the modal promises a draw "from the reviewer
// list", so handing a person's slot to a channel-wide mention would break that.
func DrawReplacementReviewer(db *gorm.DB, task models.ReviewTask, requireReviewerList bool) (string, bool) {
	labelName := task.LabelName
	if labelName == "" {
		labelName = "needs-review"
	}

	excludeIDs := TaskReviewerIDs(task)
	if task.PRAuthorSlackID != "" {
		excludeIDs = append(excludeIDs, task.PRAuthorSlackID)
	}

	picked := SelectRandomReviewers(db, task.SlackChannel, labelName, 1, excludeIDs)
	if len(picked) == 0 {
		return "", false
	}

	// SelectRandomReviewers falls back to DefaultMentionID when every listed
	// reviewer is excluded; that fallback is a channel-wide mention, not a
	// person who can take over the review.
	var cfg models.ChannelConfig
	if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&cfg).Error; err == nil {
		if (cfg.ReviewerList != "" || requireReviewerList) && picked[0] == cfg.DefaultMentionID {
			return "", false
		}
	}
	return picked[0], true
}

// TaskReviewerIDs returns the reviewers currently assigned to the task,
// falling back to the legacy single Reviewer field.
// Duplicates are dropped: radio_buttons rejects options sharing a value.
func TaskReviewerIDs(task models.ReviewTask) []string {
	var ids []string
	seen := map[string]bool{}
	for _, id := range strings.Split(task.Reviewers, ",") {
		if trimmed := strings.TrimSpace(id); trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 && strings.TrimSpace(task.Reviewer) != "" {
		ids = append(ids, strings.TrimSpace(task.Reviewer))
	}
	return ids
}

// HasMultipleReviewers reports whether the task should use the modal flow
// (pick who to replace) instead of the one-click random re-draw.
// Above the radio_buttons cap the modal cannot render, so those tasks keep the
// random re-draw rather than opening a view Slack would reject.
func HasMultipleReviewers(task models.ReviewTask) bool {
	n := len(TaskReviewerIDs(task))
	return n >= 2 && n <= slackRadioButtonsMaxOptions
}

// ReplaceTaskReviewer swaps oldID for newID in the task's reviewer fields.
func ReplaceTaskReviewer(task *models.ReviewTask, oldID, newID string) {
	if task.Reviewers != "" {
		var updated []string
		for _, id := range strings.Split(task.Reviewers, ",") {
			trimmed := strings.TrimSpace(id)
			if trimmed == oldID {
				trimmed = newID
			}
			if trimmed != "" {
				updated = append(updated, trimmed)
			}
		}
		task.Reviewers = strings.Join(updated, ",")
	}
	if task.Reviewers == "" || task.Reviewer == oldID {
		task.Reviewer = newID
	}
}

var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeMrkdwn(s string) string {
	return mrkdwnEscaper.Replace(s)
}

// BuildChangeReviewerModalView renders the modal. The target is picked with
// radio_buttons because, unlike static_select, their option text accepts mrkdwn,
// so "<@U…>" renders as the reviewer's name instead of a raw user id.
func BuildChangeReviewerModalView(task models.ReviewTask) map[string]any {
	t := i18n.L(task.Language)

	plainText := func(s string) map[string]any {
		return map[string]any{"type": "plain_text", "text": s}
	}

	var options []map[string]any
	for _, id := range TaskReviewerIDs(task) {
		options = append(options, map[string]any{
			"text":  map[string]any{"type": "mrkdwn", "text": "<@" + id + ">"},
			"value": id,
		})
	}

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": t("modal.change_reviewer.header", escapeMrkdwn(task.Title), task.PRURL),
			},
		},
		{
			"type":     "input",
			"block_id": ChangeReviewerTargetBlockID,
			"label":    plainText(t("modal.change_reviewer.target")),
			"element": map[string]any{
				"type":      "radio_buttons",
				"action_id": ChangeReviewerTargetBlockID,
				"options":   options,
			},
		},
		{
			"type":     "input",
			"block_id": ChangeReviewerNewBlockID,
			"optional": true,
			"label":    plainText(t("modal.change_reviewer.new")),
			"hint":     plainText(t("modal.change_reviewer.new.hint")),
			"element": map[string]any{
				"type":      "users_select",
				"action_id": ChangeReviewerNewBlockID,
			},
		},
	}

	return map[string]any{
		"type":             "modal",
		"callback_id":      ChangeReviewerModalCallbackID,
		"private_metadata": EncodeChangeReviewerModalMetadata(ChangeReviewerModalMetadata{TaskID: task.ID}),
		"title":            plainText(t("modal.change_reviewer.title")),
		"submit":           plainText(t("modal.change_reviewer.submit")),
		"close":            plainText(t("modal.change_reviewer.close")),
		"blocks":           blocks,
	}
}

// ParseChangeReviewerModalSubmission validates the submission against the
// task as it is now, not as it was when the modal opened: another user may have
// swapped reviewers in the meantime.
func ParseChangeReviewerModalSubmission(values map[string]map[string]ViewStateValue, task models.ReviewTask) (*ChangeReviewerForm, error) {
	t := i18n.L(task.Language)
	errs := map[string]string{}

	form := &ChangeReviewerForm{}
	if v, ok := values[ChangeReviewerTargetBlockID][ChangeReviewerTargetBlockID]; ok && v.SelectedOption != nil {
		form.OldReviewerID = v.SelectedOption.Value
	}
	if v, ok := values[ChangeReviewerNewBlockID][ChangeReviewerNewBlockID]; ok {
		form.NewReviewerID = v.SelectedUser
	}

	current := TaskReviewerIDs(task)
	isCurrent := func(id string) bool {
		for _, c := range current {
			if c == id {
				return true
			}
		}
		return false
	}

	switch {
	case form.OldReviewerID == "":
		errs[ChangeReviewerTargetBlockID] = t("modal.change_reviewer.error.target_required")
	case !isCurrent(form.OldReviewerID):
		errs[ChangeReviewerTargetBlockID] = t("modal.change_reviewer.error.target_not_assigned")
	}

	switch {
	case form.NewReviewerID == "":
		// Left blank on purpose: the caller draws a successor at random.
	case !LooksLikeResolvedSlackUserID(form.NewReviewerID):
		errs[ChangeReviewerNewBlockID] = t("modal.change_reviewer.error.new_not_resolved")
	case form.NewReviewerID == task.PRAuthorSlackID:
		errs[ChangeReviewerNewBlockID] = t("modal.change_reviewer.error.new_is_author")
	case isCurrent(form.NewReviewerID):
		errs[ChangeReviewerNewBlockID] = t("modal.change_reviewer.error.new_already_assigned")
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}
