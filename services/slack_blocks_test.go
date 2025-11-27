package services

import (
	"reflect"
	"testing"
)

func TestNewSlackBlockBuilder(t *testing.T) {
	builder := NewSlackBlockBuilder()

	if builder == nil {
		t.Fatal("builder should not be nil")
		return
	}

	if len(builder.blocks) != 0 {
		t.Error("initial blocks should be empty")
	}
}

func TestSlackBlockBuilder_AddSection(t *testing.T) {
	builder := NewSlackBlockBuilder()
	result := builder.AddSection("test message")

	// チェーン可能性の確認
	if result != builder {
		t.Error("AddSection should return builder for chaining")
	}

	blocks := builder.Build()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	block := blocks[0]
	expectedBlock := map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": "test message",
		},
	}

	if !reflect.DeepEqual(block, expectedBlock) {
		t.Errorf("expected %+v, got %+v", expectedBlock, block)
	}
}

func TestSlackBlockBuilder_AddActions(t *testing.T) {
	builder := NewSlackBlockBuilder()
	button := CreateButton("Test Button", "test_action", "test_value", "")

	result := builder.AddActions(button)

	// チェーン可能性の確認
	if result != builder {
		t.Error("AddActions should return builder for chaining")
	}

	blocks := builder.Build()
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	block := blocks[0]
	expectedBlock := map[string]interface{}{
		"type":     "actions",
		"elements": []map[string]interface{}{button},
	}

	if !reflect.DeepEqual(block, expectedBlock) {
		t.Errorf("expected %+v, got %+v", expectedBlock, block)
	}
}

func TestSlackBlockBuilder_AddActions_NoElements(t *testing.T) {
	builder := NewSlackBlockBuilder()
	result := builder.AddActions()

	// 要素がない場合はbuilderが変更されないことを確認
	if result != builder {
		t.Error("AddActions should return builder for chaining")
	}

	blocks := builder.Build()
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks when no elements provided, got %d", len(blocks))
	}
}

func TestSlackBlockBuilder_Chaining(t *testing.T) {
	button := CreateButton("Test", "test", "value", "")
	blocks := NewSlackBlockBuilder().
		AddSection("Section 1").
		AddSection("Section 2").
		AddActions(button).
		Build()

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	// 最初のセクション
	expectedSection1 := map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": "Section 1",
		},
	}
	if !reflect.DeepEqual(blocks[0], expectedSection1) {
		t.Errorf("first block mismatch: expected %+v, got %+v", expectedSection1, blocks[0])
	}

	// 2番目のセクション
	expectedSection2 := map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": "Section 2",
		},
	}
	if !reflect.DeepEqual(blocks[1], expectedSection2) {
		t.Errorf("second block mismatch: expected %+v, got %+v", expectedSection2, blocks[1])
	}

	// アクションブロック
	expectedActions := map[string]interface{}{
		"type":     "actions",
		"elements": []map[string]interface{}{button},
	}
	if !reflect.DeepEqual(blocks[2], expectedActions) {
		t.Errorf("actions block mismatch: expected %+v, got %+v", expectedActions, blocks[2])
	}
}

func TestCreateButton(t *testing.T) {
	button := CreateButton("Click Me", "click_action", "click_value", "primary")

	expected := map[string]interface{}{
		"type": "button",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": "Click Me",
		},
		"action_id": "click_action",
		"value":     "click_value",
		"style":     "primary",
	}

	if !reflect.DeepEqual(button, expected) {
		t.Errorf("expected %+v, got %+v", expected, button)
	}
}

func TestCreateButton_NoStyle(t *testing.T) {
	button := CreateButton("Click Me", "click_action", "click_value", "")

	expected := map[string]interface{}{
		"type": "button",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": "Click Me",
		},
		"action_id": "click_action",
		"value":     "click_value",
	}

	if !reflect.DeepEqual(button, expected) {
		t.Errorf("expected %+v, got %+v", expected, button)
	}
}

func TestCreateButton_EmptyValue(t *testing.T) {
	button := CreateButton("Click Me", "click_action", "", "")

	expected := map[string]interface{}{
		"type": "button",
		"text": map[string]interface{}{
			"type": "plain_text",
			"text": "Click Me",
		},
		"action_id": "click_action",
		"value":     "default",
	}

	if !reflect.DeepEqual(button, expected) {
		t.Errorf("expected %+v, got %+v", expected, button)
	}
}

func TestCreatePauseReminderSelect(t *testing.T) {
	options := []PauseOption{
		{Text: "1時間停止", Value: "1h"},
		{Text: "完全停止", Value: "stop"},
	}

	selectElement := CreatePauseReminderSelect("task123", "pause_action", "選択してください", options)

	expected := map[string]interface{}{
		"type": "static_select",
		"placeholder": map[string]interface{}{
			"type": "plain_text",
			"text": "選択してください",
		},
		"action_id": "pause_action",
		"options": []map[string]interface{}{
			{
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": "1時間停止",
				},
				"value": "task123:1h",
			},
			{
				"text": map[string]interface{}{
					"type": "plain_text",
					"text": "完全停止",
				},
				"value": "task123:stop",
			},
		},
	}

	if !reflect.DeepEqual(selectElement, expected) {
		t.Errorf("expected %+v, got %+v", expected, selectElement)
	}
}

func TestCreateAllOptionsPauseReminderSelect(t *testing.T) {
	selectElement := CreateAllOptionsPauseReminderSelect("task123", "pause_action", "選択...")

	// AllPauseOptionsが正しく使われているかチェック
	options, ok := selectElement["options"].([]map[string]interface{})
	if !ok {
		t.Fatal("options should be a slice of maps")
	}

	if len(options) != len(AllPauseOptions) {
		t.Errorf("expected %d options, got %d", len(AllPauseOptions), len(options))
	}

	// 最初のオプションをチェック
	firstOption := options[0]
	expectedValue := "task123:" + AllPauseOptions[0].Value
	if firstOption["value"] != expectedValue {
		t.Errorf("expected first option value %s, got %v", expectedValue, firstOption["value"])
	}
}

func TestCreateStopOnlyPauseReminderSelect(t *testing.T) {
	selectElement := CreateStopOnlyPauseReminderSelect("task123", "pause_action", "完全停止")

	options, ok := selectElement["options"].([]map[string]interface{})
	if !ok {
		t.Fatal("options should be a slice of maps")
	}

	if len(options) != 1 {
		t.Errorf("expected 1 option for stop-only select, got %d", len(options))
	}

	expectedValue := "task123:stop"
	if options[0]["value"] != expectedValue {
		t.Errorf("expected value %s, got %v", expectedValue, options[0]["value"])
	}
}

func TestCreateChangeReviewerButton(t *testing.T) {
	button := CreateChangeReviewerButton("task123")

	expected := CreateButton("変わってほしい！", "change_reviewer", "task123", "danger")

	if !reflect.DeepEqual(button, expected) {
		t.Errorf("expected %+v, got %+v", expected, button)
	}
}

func TestCreateMessageBlocks(t *testing.T) {
	message := "テストメッセージ"
	blocks := CreateMessageBlocks(message)

	expected := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": "テストメッセージ",
			},
		},
	}

	if !reflect.DeepEqual(blocks, expected) {
		t.Errorf("expected %+v, got %+v", expected, blocks)
	}
}

func TestCreateMessageWithActionBlocks(t *testing.T) {
	message := "アクション付きメッセージ"
	button := CreateButton("クリック", "click_action", "value", "")
	blocks := CreateMessageWithActionBlocks(message, button)

	expected := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": "アクション付きメッセージ",
			},
		},
		{
			"type":     "actions",
			"elements": []map[string]interface{}{button},
		},
	}

	if !reflect.DeepEqual(blocks, expected) {
		t.Errorf("expected %+v, got %+v", expected, blocks)
	}
}

func TestCreateMessageWithActionsBlocks(t *testing.T) {
	message := "複数アクション付きメッセージ"
	button1 := CreateButton("ボタン1", "action1", "value1", "")
	button2 := CreateButton("ボタン2", "action2", "value2", "primary")
	blocks := CreateMessageWithActionsBlocks(message, button1, button2)

	expected := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": "複数アクション付きメッセージ",
			},
		},
		{
			"type":     "actions",
			"elements": []map[string]interface{}{button1, button2},
		},
	}

	if !reflect.DeepEqual(blocks, expected) {
		t.Errorf("expected %+v, got %+v", expected, blocks)
	}
}

func TestCreateMessageBlocks_EmptyMessage(t *testing.T) {
	blocks := CreateMessageBlocks("")

	expected := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": " ",
			},
		},
	}

	if !reflect.DeepEqual(blocks, expected) {
		t.Errorf("expected %+v, got %+v", expected, blocks)
	}
}

func TestCreatePauseReminderSelect_EmptyValues(t *testing.T) {
	options := []PauseOption{
		{Text: "停止", Value: "stop"},
	}

	selectElement := CreatePauseReminderSelect("", "", "", options)

	// taskIDとplaceholderにデフォルト値が設定されることを確認
	placeholderObj := selectElement["placeholder"].(map[string]interface{})
	if placeholderObj["text"] != "選択してください" {
		t.Errorf("expected default placeholder, got %v", placeholderObj["text"])
	}

	optionsArray := selectElement["options"].([]map[string]interface{})
	expectedValue := "unknown:stop"
	if optionsArray[0]["value"] != expectedValue {
		t.Errorf("expected %s, got %v", expectedValue, optionsArray[0]["value"])
	}
}
