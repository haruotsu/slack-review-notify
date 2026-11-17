package services

import (
	"encoding/json"
	"slack-review-notify/i18n"
	"strings"
)

// MenuModalCallbackID identifies the menu modal, the hub every other modal and
// read-only result view is reached from.
const MenuModalCallbackID = "menu_modal"

// MenuResultCallbackID identifies the read-only views pushed on top of the menu.
const MenuResultCallbackID = "menu_result"

// OpenMenuActionID is the action_id of the help-command button that opens the menu.
const OpenMenuActionID = "open_menu"

// MenuActionPrefix prefixes the action_id of every button inside the menu; the
// remainder is one of the MenuOp* values.
const MenuActionPrefix = "menu:"

// MenuUnsetAwayActionID is the action_id of the per-row button in the leave
// list. The button's value carries the ReviewerAvailability ID to remove.
const MenuUnsetAwayActionID = "menu_unset_away"

const (
	MenuOpShowLabels       = "show_labels"
	MenuOpShowLabelDetails = "show_label_details"
	MenuOpShowMyReviews    = "show_my_reviews"
	MenuOpShowReviewers    = "show_reviewers"
	MenuOpShowAvailability = "show_availability"
	MenuOpShowUserMappings = "show_user_mappings"
	MenuOpEditSettings     = "edit_settings"
	MenuOpNewLabel         = "new_label"
	MenuOpAway             = "away"
	MenuOpUserMapping      = "user_mapping"
)

// MenuShowOps (read-only results) and MenuConfigOps (editing modals) list the
// menu buttons in display order.
var (
	MenuShowOps = []string{
		MenuOpShowLabels, MenuOpShowLabelDetails, MenuOpShowMyReviews,
		MenuOpShowReviewers, MenuOpShowAvailability, MenuOpShowUserMappings,
	}
	MenuConfigOps = []string{
		MenuOpEditSettings, MenuOpNewLabel, MenuOpAway, MenuOpUserMapping,
	}
)

// MenuModalMetadata round-trips through view.private_metadata. Buttons inside a
// modal arrive without a channel in the payload's container, so the channel the
// menu was opened from has to travel with the view.
type MenuModalMetadata struct {
	ChannelID string `json:"c"`
	UserID    string `json:"u"`
}

func EncodeMenuModalMetadata(m MenuModalMetadata) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func DecodeMenuModalMetadata(s string) (MenuModalMetadata, error) {
	var m MenuModalMetadata
	if s == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return m, err
	}
	return m, nil
}

// MenuOpLabel returns the button label for a menu operation. The same text
// titles the view the button opens.
func MenuOpLabel(op, lang string) string {
	return i18n.TWithLang(lang, "modal.menu.op."+op)
}

func menuPlainText(s string) map[string]any {
	return map[string]any{"type": "plain_text", "text": s}
}

func menuSection(text string) map[string]any {
	return map[string]any{
		"type": "section",
		"text": map[string]any{"type": "mrkdwn", "text": text},
	}
}

// BuildMenuModalView renders the menu: one button per operation, grouped into
// read-only views and editing modals. It has no submit button because every
// operation is carried out by the view its button opens.
func BuildMenuModalView(meta MenuModalMetadata, lang string) map[string]any {
	t := i18n.L(lang)

	buttons := func(ops []string) []map[string]any {
		elements := make([]map[string]any, 0, len(ops))
		for _, op := range ops {
			elements = append(elements, CreateButton(MenuOpLabel(op, lang), MenuActionPrefix+op, op, ""))
		}
		return elements
	}

	blocks := []map[string]any{
		menuSection(t("modal.menu.header")),
		menuSection(t("modal.menu.section.view")),
		{"type": "actions", "block_id": "menu_view", "elements": buttons(MenuShowOps)},
		menuSection(t("modal.menu.section.edit")),
		{"type": "actions", "block_id": "menu_edit", "elements": buttons(MenuConfigOps)},
	}

	return map[string]any{
		"type":             "modal",
		"callback_id":      MenuModalCallbackID,
		"private_metadata": EncodeMenuModalMetadata(meta),
		"title":            menuPlainText(t("modal.menu.title")),
		"close":            menuPlainText(t("modal.menu.close")),
		"blocks":           blocks,
	}
}

// BuildMenuResultView renders the read-only view shown for a "show" operation.
// Each entry of texts becomes its own run of sections, separated by dividers.
func BuildMenuResultView(meta MenuModalMetadata, lang, title string, texts []string) map[string]any {
	var blocks []map[string]any
	for _, text := range texts {
		chunks := SplitTextForSections(text, SlackSectionTextLimit)
		if len(chunks) == 0 {
			continue
		}
		if len(blocks) > 0 {
			blocks = append(blocks, map[string]any{"type": "divider"})
		}
		for _, chunk := range chunks {
			// Splitting on a line boundary can leave a chunk of nothing but the
			// trailing newline, and Slack rejects the whole view over one empty
			// text object.
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			blocks = append(blocks, menuSection(chunk))
		}
	}
	return menuResultView(meta, lang, title, blocks)
}

// MenuAvailabilityRow is one leave period in the leave list.
type MenuAvailabilityRow struct {
	ID   string
	Text string
}

// BuildMenuAvailabilityView renders the leave list with a remove button per
// period, which is what lets a single period be cancelled without typing its
// dates back in.
func BuildMenuAvailabilityView(meta MenuModalMetadata, lang string, rows []MenuAvailabilityRow) map[string]any {
	t := i18n.L(lang)
	title := MenuOpLabel(MenuOpShowAvailability, lang)

	if len(rows) == 0 {
		return menuResultView(meta, lang, title, []map[string]any{menuSection(t("cmd.show_availability.empty"))})
	}

	// The header takes one block; the truncation note takes another, but only
	// when something is actually left out.
	shown := rows
	if len(shown) > SlackModalMaxBlocks-1 {
		shown = shown[:SlackModalMaxBlocks-2]
	}

	blocks := []map[string]any{menuSection(t("cmd.show_availability.header"))}
	for _, row := range shown {
		button := CreateButton(t("modal.menu.availability.unset"), MenuUnsetAwayActionID, row.ID, "danger")
		button["confirm"] = map[string]any{
			"title":   menuPlainText(t("modal.menu.availability.confirm.title")),
			"text":    menuPlainText(t("modal.menu.availability.confirm.text")),
			"confirm": menuPlainText(t("modal.menu.availability.confirm.ok")),
			"deny":    menuPlainText(t("modal.menu.availability.confirm.deny")),
		}
		block := menuSection(TruncateForSlack(row.Text, SlackSectionTextLimit))
		block["accessory"] = button
		blocks = append(blocks, block)
	}
	if hidden := len(rows) - len(shown); hidden > 0 {
		blocks = append(blocks, menuSection(t("modal.menu.truncated", hidden)))
	}
	return menuResultView(meta, lang, title, blocks)
}

func menuResultView(meta MenuModalMetadata, lang, title string, blocks []map[string]any) map[string]any {
	t := i18n.L(lang)
	if len(blocks) > SlackModalMaxBlocks {
		// No count here: the blocks dropped are sections and dividers, not
		// entries, so a number would not mean anything to the reader.
		blocks = append(blocks[:SlackModalMaxBlocks-1:SlackModalMaxBlocks-1], menuSection(t("modal.menu.truncated_text")))
	}
	return map[string]any{
		"type":             "modal",
		"callback_id":      MenuResultCallbackID,
		"private_metadata": EncodeMenuModalMetadata(meta),
		"title":            menuPlainText(title),
		"close":            menuPlainText(t("modal.menu.back")),
		"blocks":           blocks,
	}
}
