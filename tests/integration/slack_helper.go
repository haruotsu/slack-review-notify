package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SendTestMessage sends a test message to the specified Slack channel
// Returns the message timestamp (ts) which can be used to delete the message later
func SendTestMessage(channelID, text string) (string, error) {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return "", fmt.Errorf("SLACK_BOT_TOKEN is not set")
	}

	body := map[string]interface{}{
		"channel": channelID,
		"text":    text,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result struct {
		OK      bool   `json:"ok"`
		TS      string `json:"ts"`
		Channel string `json:"channel"`
		Error   string `json:"error"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.TS, nil
}

// DeleteTestMessage deletes a test message from the specified Slack channel
func DeleteTestMessage(channelID, ts string) error {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN is not set")
	}

	body := map[string]interface{}{
		"channel": channelID,
		"ts":      ts,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.delete", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("slack API error: %s", result.Error)
	}

	return nil
}

// Message represents a Slack message
type Message struct {
	Type      string `json:"type"`
	User      string `json:"user"`
	Text      string `json:"text"`
	Timestamp string `json:"ts"`
	ThreadTS  string `json:"thread_ts,omitempty"`
}

// GetChannelMessages retrieves messages from the specified Slack channel
// The limit parameter specifies the maximum number of messages to retrieve
func GetChannelMessages(channelID string, limit int) ([]Message, error) {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN is not set")
	}

	url := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=%d", channelID, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result struct {
		OK       bool      `json:"ok"`
		Messages []Message `json:"messages"`
		Error    string    `json:"error"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.Messages, nil
}

// WaitForMessage waits for a message containing the expected text to appear in the channel
// Returns the message if found, or an error if the timeout is reached
func WaitForMessage(channelID, expectedText string, timeout time.Duration) (*Message, error) {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN is not set")
	}

	startTime := time.Now()
	checkInterval := 1 * time.Second

	for {
		// Check if timeout has been reached
		if time.Since(startTime) > timeout {
			return nil, fmt.Errorf("timeout waiting for message containing: %s", expectedText)
		}

		// Get recent messages
		messages, err := GetChannelMessages(channelID, 10)
		if err != nil {
			return nil, fmt.Errorf("failed to get channel messages: %w", err)
		}

		// Check if any message contains the expected text
		for i := range messages {
			msg := &messages[i]
			if strings.Contains(msg.Text, expectedText) {
				return msg, nil
			}
		}

		// Wait before checking again
		time.Sleep(checkInterval)
	}
}
