package services

import (
	"encoding/json"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"strings"
)

// UserMappingModalCallbackID identifies the user-mapping modal in
// view_submission payloads. Routed in handlers.HandleSlackAction.
const UserMappingModalCallbackID = "user_mapping_modal"

// OpenUserMappingActionID is the action_id of the help-command button that
// opens this modal.
const OpenUserMappingActionID = "open_user_mapping"

// Block IDs are exported as constants so the parser and view builder can
// stay in sync without action-at-a-distance.
const (
	UserMappingGithubBlockID    = "user_mapping_github_username"
	UserMappingSlackUserBlockID = "user_mapping_slack_user"
	UserMappingDeleteBlockID    = "user_mapping_delete"
)

// UserMappingModalMetadata round-trips through view.private_metadata so the
// submission handler can post the result confirmation back to the user.
type UserMappingModalMetadata struct {
	ChannelID string `json:"c"`
	UserID    string `json:"u"`
}

func EncodeUserMappingModalMetadata(m UserMappingModalMetadata) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func DecodeUserMappingModalMetadata(s string) (UserMappingModalMetadata, error) {
	var m UserMappingModalMetadata
	if s == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return m, err
	}
	return m, nil
}

// UserMappingModalInputs is the BuildUserMappingModalView parameter struct.
// Mappings is the existing list, rendered read-only at the top of the modal
// so the operator can see what's already configured before adding a new row.
type UserMappingModalInputs struct {
	ChannelID string
	UserID    string
	Lang      string
	Mappings  []models.UserMapping
}

// UserMappingForm is the parsed view_submission payload.
//   - When Delete is true, the row identified by GithubUsername is removed
//     (SlackUserID is ignored).
//   - Otherwise GithubUsername+SlackUserID are upserted.
type UserMappingForm struct {
	GithubUsername string
	SlackUserID    string
	Delete         bool
}

// BuildUserMappingModalView renders the user-mapping modal. The slack user is
// chosen via users_select so the bot only ever sees a resolved U-id — the
// missing piece that allowed plain @handles into user_mappings before, and
// that broke PR-author exclusion once reviewer_list became U-id-only.
func BuildUserMappingModalView(in UserMappingModalInputs) map[string]any {
	t := i18n.L(in.Lang)

	plainText := func(s string) map[string]any {
		return map[string]any{"type": "plain_text", "text": s}
	}

	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": t("modal.user_mapping.header"),
			},
		},
	}

	if len(in.Mappings) > 0 {
		// Render existing entries as a single markdown block. Legacy non-U-id
		// rows get a ⚠️ so operators see at a glance which entries still need
		// to be re-registered via this modal.
		var b strings.Builder
		b.WriteString(t("modal.user_mapping.existing_header"))
		b.WriteString("\n")
		for _, m := range in.Mappings {
			if LooksLikeResolvedSlackUserID(m.SlackUserID) {
				b.WriteString("• `")
				b.WriteString(m.GithubUsername)
				b.WriteString("` → <@")
				b.WriteString(m.SlackUserID)
				b.WriteString(">\n")
			} else {
				// Legacy row: surface the raw stored value so the operator can
				// recognize it, and flag it.
				b.WriteString("• ⚠️ `")
				b.WriteString(m.GithubUsername)
				b.WriteString("` → `")
				b.WriteString(m.SlackUserID)
				b.WriteString("` ")
				b.WriteString(t("modal.user_mapping.legacy_flag"))
				b.WriteString("\n")
			}
		}
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": b.String(),
			},
		})
		blocks = append(blocks, map[string]any{"type": "divider"})
	}

	// GitHub username input
	blocks = append(blocks, map[string]any{
		"type":     "input",
		"block_id": UserMappingGithubBlockID,
		"label":    plainText(t("modal.user_mapping.github")),
		"hint":     plainText(t("modal.user_mapping.github.hint")),
		"element": map[string]any{
			"type":      "plain_text_input",
			"action_id": UserMappingGithubBlockID,
		},
	})

	// Slack user picker (resolved U-id only)
	blocks = append(blocks, map[string]any{
		"type":     "input",
		"block_id": UserMappingSlackUserBlockID,
		"optional": true,
		"label":    plainText(t("modal.user_mapping.slack_user")),
		"hint":     plainText(t("modal.user_mapping.slack_user.hint")),
		"element": map[string]any{
			"type":      "users_select",
			"action_id": UserMappingSlackUserBlockID,
		},
	})

	// Delete checkbox
	blocks = append(blocks, map[string]any{
		"type":     "input",
		"block_id": UserMappingDeleteBlockID,
		"optional": true,
		"label":    plainText(t("modal.user_mapping.delete")),
		"hint":     plainText(t("modal.user_mapping.delete.hint")),
		"element": map[string]any{
			"type":      "checkboxes",
			"action_id": UserMappingDeleteBlockID,
			"options": []map[string]any{
				{
					"text":  plainText(t("modal.user_mapping.delete.option")),
					"value": "yes",
				},
			},
		},
	})

	return map[string]any{
		"type":        "modal",
		"callback_id": UserMappingModalCallbackID,
		"private_metadata": EncodeUserMappingModalMetadata(UserMappingModalMetadata{
			ChannelID: in.ChannelID,
			UserID:    in.UserID,
		}),
		"title":  plainText(t("modal.user_mapping.title")),
		"submit": plainText(t("modal.user_mapping.submit")),
		"close":  plainText(t("modal.user_mapping.close")),
		"blocks": blocks,
	}
}

// ParseUserMappingModalSubmission converts the view.state.values map into a
// UserMappingForm. Validation:
//   - GithubUsername must be non-empty.
//   - When Delete is false, SlackUserID must be a resolved Slack user id.
//     The users_select picker already guarantees this in normal Slack flows,
//     but we validate defensively so a synthesized payload can't bypass the
//     guard that exists to prevent the original PR-author-as-reviewer bug.
func ParseUserMappingModalSubmission(values map[string]map[string]ViewStateValue, lang string) (*UserMappingForm, error) {
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

	form := &UserMappingForm{}
	form.GithubUsername = field(UserMappingGithubBlockID)
	form.SlackUserID = selectedUser(UserMappingSlackUserBlockID)
	form.Delete = checkboxChecked(UserMappingDeleteBlockID, "yes")

	if form.GithubUsername == "" {
		errs[UserMappingGithubBlockID] = t("modal.user_mapping.error.github_required")
	}
	if !form.Delete {
		if form.SlackUserID == "" {
			errs[UserMappingSlackUserBlockID] = t("modal.user_mapping.error.slack_user_required")
		} else if !LooksLikeResolvedSlackUserID(form.SlackUserID) {
			errs[UserMappingSlackUserBlockID] = t("modal.user_mapping.error.slack_user_not_resolved")
		}
	}

	if len(errs) > 0 {
		return nil, &ModalValidationError{Errors: errs}
	}
	return form, nil
}
