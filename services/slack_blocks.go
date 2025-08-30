package services

import "fmt"

// PauseOption リマインダー停止オプションの構造体
type PauseOption struct {
	Text  string
	Value string
}

// 共通のリマインダー停止オプション
var (
	PauseOptionOneHour   = PauseOption{Text: "1時間停止", Value: "1h"}
	PauseOptionTwoHours  = PauseOption{Text: "2時間停止", Value: "2h"}
	PauseOptionFourHours = PauseOption{Text: "4時間停止", Value: "4h"}
	PauseOptionToday     = PauseOption{Text: "今日は通知しない (翌営業日の朝まで停止)", Value: "today"}
	PauseOptionStop      = PauseOption{Text: "リマインダーを完全に停止", Value: "stop"}
)

// AllPauseOptions 全てのリマインダー停止オプション
var AllPauseOptions = []PauseOption{
	PauseOptionOneHour,
	PauseOptionTwoHours,
	PauseOptionFourHours,
	PauseOptionToday,
	PauseOptionStop,
}

// SlackBlockBuilder Slack Block Kit構築のヘルパー
type SlackBlockBuilder struct {
	blocks []map[string]interface{}
}

// NewSlackBlockBuilder 新しいビルダーを作成
func NewSlackBlockBuilder() *SlackBlockBuilder {
	return &SlackBlockBuilder{
		blocks: make([]map[string]interface{}, 0),
	}
}

// AddSection セクションブロックを追加
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

// AddActions アクションブロックを追加
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

// Build ブロック配列を取得
func (b *SlackBlockBuilder) Build() []map[string]interface{} {
	return b.blocks
}

// CreateButton ボタン要素を作成
func CreateButton(text, actionID, value, style string) map[string]interface{} {
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

// CreatePauseReminderSelect リマインダー停止セレクトを作成
func CreatePauseReminderSelect(taskID, actionID, placeholder string, options []PauseOption) map[string]interface{} {
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

// CreateChangeReviewerButton レビュワー変更ボタンを作成
func CreateChangeReviewerButton(taskID string) map[string]interface{} {
	return CreateButton("変わってほしい！", "change_reviewer", taskID, "danger")
}

// CreateAllOptionsPauseReminderSelect 全オプション付きリマインダー停止セレクトを作成
func CreateAllOptionsPauseReminderSelect(taskID, actionID, placeholder string) map[string]interface{} {
	return CreatePauseReminderSelect(taskID, actionID, placeholder, AllPauseOptions)
}

// CreateStopOnlyPauseReminderSelect 完全停止のみのリマインダー停止セレクトを作成
func CreateStopOnlyPauseReminderSelect(taskID, actionID, placeholder string) map[string]interface{} {
	return CreatePauseReminderSelect(taskID, actionID, placeholder, []PauseOption{PauseOptionStop})
}

// CreateMessageBlocks メッセージのみのブロックを作成
func CreateMessageBlocks(message string) []map[string]interface{} {
	return NewSlackBlockBuilder().
		AddSection(message).
		Build()
}

// CreateMessageWithActionBlocks メッセージと1つのアクションのブロックを作成
func CreateMessageWithActionBlocks(message string, action map[string]interface{}) []map[string]interface{} {
	return NewSlackBlockBuilder().
		AddSection(message).
		AddActions(action).
		Build()
}

// CreateMessageWithActionsBlocks メッセージと複数のアクションのブロックを作成
func CreateMessageWithActionsBlocks(message string, actions ...map[string]interface{}) []map[string]interface{} {
	return NewSlackBlockBuilder().
		AddSection(message).
		AddActions(actions...).
		Build()
}