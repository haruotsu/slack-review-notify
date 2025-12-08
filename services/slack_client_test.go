package services

import (
	"testing"

	"slack-review-notify/models"
)

// TestMockSlackClient はモッククライアントが正しく動作することを確認するテスト
func TestMockSlackClient(t *testing.T) {
	// モッククライアントを作成
	mockClient := NewMockSlackClient()

	// SetSlackClient でモックを設定
	originalClient := GetSlackClient()
	SetSlackClient(mockClient)
	defer SetSlackClient(originalClient) // テスト終了後に元に戻す

	// SendSlackMessage を呼び出す
	ts, channelID, err := mockClient.SendSlackMessage(
		"https://github.com/test/repo/pull/1",
		"Test PR",
		"C1234567890",
		"U1234567890",
		"U0987654321",
	)

	// 結果を確認
	if err != nil {
		t.Errorf("SendSlackMessage should not return error, got: %v", err)
	}

	if ts != "1234567890.123456" {
		t.Errorf("expected ts to be '1234567890.123456', got: %s", ts)
	}

	if channelID != "C1234567890" {
		t.Errorf("expected channelID to be 'C1234567890', got: %s", channelID)
	}

	// 呼び出しが記録されているか確認
	if len(mockClient.SendMessageCalls) != 1 {
		t.Errorf("expected 1 call to SendSlackMessage, got: %d", len(mockClient.SendMessageCalls))
	}

	call := mockClient.SendMessageCalls[0]
	if call.PRURL != "https://github.com/test/repo/pull/1" {
		t.Errorf("expected PRURL to be 'https://github.com/test/repo/pull/1', got: %s", call.PRURL)
	}

	if call.Title != "Test PR" {
		t.Errorf("expected Title to be 'Test PR', got: %s", call.Title)
	}
}

// TestMockSlackClientPostToThread はPostToThreadメソッドのモックテスト
func TestMockSlackClientPostToThread(t *testing.T) {
	mockClient := NewMockSlackClient()

	// SetSlackClient でモックを設定
	originalClient := GetSlackClient()
	SetSlackClient(mockClient)
	defer SetSlackClient(originalClient)

	err := mockClient.PostToThread("C1234567890", "1234567890.123456", "Test message")

	if err != nil {
		t.Errorf("PostToThread should not return error, got: %v", err)
	}

	// 呼び出しが記録されているか確認
	if len(mockClient.PostToThreadCalls) != 1 {
		t.Errorf("expected 1 call to PostToThread, got: %d", len(mockClient.PostToThreadCalls))
	}

	call := mockClient.PostToThreadCalls[0]
	if call.Channel != "C1234567890" {
		t.Errorf("expected Channel to be 'C1234567890', got: %s", call.Channel)
	}

	if call.Message != "Test message" {
		t.Errorf("expected Message to be 'Test message', got: %s", call.Message)
	}
}

// TestMockSlackClientUpdateSlackMessageForCompletedTask はUpdateSlackMessageForCompletedTaskメソッドのモックテスト
func TestMockSlackClientUpdateSlackMessageForCompletedTask(t *testing.T) {
	mockClient := NewMockSlackClient()

	// SetSlackClient でモックを設定
	originalClient := GetSlackClient()
	SetSlackClient(mockClient)
	defer SetSlackClient(originalClient)

	task := models.ReviewTask{
		ID:           "test-task-id",
		PRURL:        "https://github.com/test/repo/pull/1",
		Title:        "Test PR",
		SlackChannel: "C1234567890",
		SlackTS:      "1234567890.123456",
	}

	err := mockClient.UpdateSlackMessageForCompletedTask(task)

	if err != nil {
		t.Errorf("UpdateSlackMessageForCompletedTask should not return error, got: %v", err)
	}

	// 呼び出しが記録されているか確認
	if len(mockClient.UpdateSlackMessageForCompletedCalls) != 1 {
		t.Errorf("expected 1 call to UpdateSlackMessageForCompletedTask, got: %d", len(mockClient.UpdateSlackMessageForCompletedCalls))
	}

	call := mockClient.UpdateSlackMessageForCompletedCalls[0]
	if call.Task.ID != "test-task-id" {
		t.Errorf("expected Task.ID to be 'test-task-id', got: %s", call.Task.ID)
	}
}

// TestRealSlackClientInterface は RealSlackClient が SlackClient インターフェースを満たすことを確認
func TestRealSlackClientInterface(t *testing.T) {
	var _ SlackClient = &RealSlackClient{}
	_ = NewRealSlackClient()
}

// TestMockSlackClientInterface は MockSlackClient が SlackClient インターフェースを満たすことを確認
func TestMockSlackClientInterface(t *testing.T) {
	var _ SlackClient = &MockSlackClient{}
	_ = NewMockSlackClient()
}

// TestGetSlackClient は GetSlackClient が正しく動作することを確認
func TestGetSlackClient(t *testing.T) {
	client := GetSlackClient()
	if client == nil {
		t.Error("GetSlackClient should not return nil")
	}
}

// TestSetSlackClient は SetSlackClient が正しく動作することを確認
func TestSetSlackClient(t *testing.T) {
	originalClient := GetSlackClient()
	defer SetSlackClient(originalClient)

	mockClient := NewMockSlackClient()
	SetSlackClient(mockClient)

	currentClient := GetSlackClient()
	if currentClient != mockClient {
		t.Error("SetSlackClient should set the client correctly")
	}
}
