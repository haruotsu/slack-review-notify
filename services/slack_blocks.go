package services

import (
	"fmt"

	"slack-review-notify/i18n"
)

// PauseOption is a struct for reminder pause options
type PauseOption struct {
	Text  string
	Value string
}

// GetAllPauseOptions returns all pause options with translated text
func GetAllPauseOptions(lang string) []PauseOption {
	t := i18n.L(lang)
	return []PauseOption{
		{Text: t("pause.1h"), Value: "1h"},
		{Text: t("pause.2h"), Value: "2h"},
		{Text: t("pause.4h"), Value: "4h"},
		{Text: t("pause.today"), Value: "today"},
		{Text: t("pause.stop"), Value: "stop"},
	}
}

// GetStopOnlyPauseOption returns the stop-only pause option with translated text
func GetStopOnlyPauseOption(lang string) PauseOption {
	return PauseOption{Text: i18n.TWithLang(lang, "pause.stop"), Value: "stop"}
}

// SlackBlockBuilder is a helper for building Slack Block Kit structures
type SlackBlockBuilder struct {
	blocks []map[string]interface{}
}

// NewSlackBlockBuilder creates a new builder
func NewSlackBlockBuilder() *SlackBlockBuilder {
	return &SlackBlockBuilder{
		blocks: make([]map[string]interface{}, 0),
	}
}

// AddSection adds a section block
func (b *SlackBlockBuilder) AddSection(text string) *SlackBlockBuilder {
	section := map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": text,
		},
	}
	b.blocks = append(b.blocks, section)
	return b
}

// AddActions adds an actions block
func (b *SlackBlockBuilder) AddActions(elements ...map[string]interface{}) *SlackBlockBuilder {
	if len(elements) == 0 {
		return b
	}

	actions := map[string]interface{}{
		"type":     "actions",
		"elements": elements,
	}
	b.blocks = append(b.blocks, actions)
	return b
}

// Build returns the block array
func (b *SlackBlockBuilder) Build() []map[string]interface{} {
	return b.blocks
}

// CreateButton creates a button element
func CreateButton(text, actionID, value, style string) map[string]interface{} {
	if value == "" {
		value = "default"
	}

	button := map[string]interface{}{
		"type": "button",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": text,
		},
		"action_id": actionID,
		"value":     value,
	}

	if style != "" {
		button["style"] = style
	}

	return button
}

// CreatePauseReminderSelect creates a reminder pause select element
func CreatePauseReminderSelect(taskID, actionID, placeholder, lang string, options []PauseOption) map[string]interface{} {
	if taskID == "" {
		taskID = "unknown"
	}
	if placeholder == "" {
		placeholder = i18n.TWithLang(lang, "common.select_placeholder")
	}

	selectOptions := make([]map[string]interface{}, len(options))
	for i, option := range options {
		selectOptions[i] = map[string]interface{}{
			"text": map[string]interface{}{
				"type": "plain_text",
				"text": option.Text,
			},
			"value": fmt.Sprintf("%s:%s", taskID, option.Value),
		}
	}

	return map[string]interface{}{
		"type": "static_select",
		"placeholder": map[string]interface{}{
			"type": "plain_text",
			"text": placeholder,
		},
		"action_id": actionID,
		"options":   selectOptions,
	}
}

// CreateChangeReviewerButton creates a change reviewer button
func CreateChangeReviewerButton(taskID, lang string) map[string]interface{} {
	return CreateButton(i18n.TWithLang(lang, "button.change_reviewer"), "change_reviewer", taskID, "danger")
}

// CreateAllOptionsPauseReminderSelect creates a reminder pause select with all options
func CreateAllOptionsPauseReminderSelect(taskID, actionID, placeholder, lang string) map[string]interface{} {
	return CreatePauseReminderSelect(taskID, actionID, placeholder, lang, GetAllPauseOptions(lang))
}

// CreateStopOnlyPauseReminderSelect creates a reminder pause select with only the full stop option
func CreateStopOnlyPauseReminderSelect(taskID, actionID, placeholder, lang string) map[string]interface{} {
	return CreatePauseReminderSelect(taskID, actionID, placeholder, lang, []PauseOption{GetStopOnlyPauseOption(lang)})
}

// CreateMessageBlocks creates blocks with message only
func CreateMessageBlocks(message string) []map[string]interface{} {
	if message == "" {
		message = " "
	}
	return NewSlackBlockBuilder().
		AddSection(message).
		Build()
}

// CreateMessageWithActionBlocks creates blocks with a message and one action
func CreateMessageWithActionBlocks(message string, action map[string]interface{}) []map[string]interface{} {
	if message == "" {
		message = " "
	}
	return NewSlackBlockBuilder().
		AddSection(message).
		AddActions(action).
		Build()
}

// CreateMessageWithActionsBlocks creates blocks with a message and multiple actions
func CreateMessageWithActionsBlocks(message string, actions ...map[string]interface{}) []map[string]interface{} {
	if message == "" {
		message = " "
	}
	return NewSlackBlockBuilder().
		AddSection(message).
		AddActions(actions...).
		Build()
}
