package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slack-review-notify/models"
	"strings"

	"gorm.io/gorm"
)

type SlackMessage struct {
    Channel string       `json:"channel"`
    Blocks  []Block      `json:"blocks"`
}

type Block struct {
    Type string          `json:"type"`
    Text *TextObject     `json:"text,omitempty"`
    Elements []Button    `json:"elements,omitempty"`
}

type TextObject struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

type Button struct {
    Type     string `json:"type"`
    Text     TextObject `json:"text"`
    ActionID string `json:"action_id"`
    Style    string `json:"style,omitempty"`
}

type SlackPostResponse struct {
    OK      bool   `json:"ok"`
    Channel string `json:"channel"`
    Ts      string `json:"ts"`
    Error   string `json:"error,omitempty"`
}

func SendSlackMessage(prURL, title, channel, mentionID string) (string, string, error) {
    blocks := []Block{
        {
            Type: "section",
            Text: &TextObject{
                Type: "mrkdwn",
                Text: fmt.Sprintf("<@%s> *レビュー対象のPRがあります！*\n\n*タイトル*: %s\n*リンク*: <%s>", mentionID, title, prURL),
            },
        },
        {
            Type: "actions",
            Elements: []Button{
                {
                    Type: "button",
                    Text: TextObject{
                        Type: "plain_text",
                        Text: "レビュー完了",
                    },
                    ActionID: "review_done",
                    Style:    "primary",
                },
            },
        },
    }

    message := SlackMessage{
        Channel: channel,
        Blocks:  blocks,
    }

    jsonData, _ := json.Marshal(message)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return "", "", err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("slack response:", string(bodyBytes))

    var slackResp SlackPostResponse
    if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
        return "", "", err
    }

    if !slackResp.OK {
        return "", "", fmt.Errorf("Slack error: %s", slackResp.Error)
    }


	return slackResp.Ts, slackResp.Channel, nil
}

// スレッドにメッセージを投稿する関数
func PostToThread(channel, ts, message string) error {
    body := map[string]interface{}{
        "channel": channel,
        "thread_ts": ts,
        "text": message,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return fmt.Errorf("slack API response parse error: %v", err)
    }
    
    if !result.OK {
        return fmt.Errorf("slack error: %s", result.Error)
    }
    
    return nil
}

// レビュー担当者が決まった時のメッセージ
func SendReviewerAssignedMessage(task models.ReviewTask) error {
    message := fmt.Sprintf("🤖 レビュアーリストからランダムに選ばれた <@%s> さんが担当になりました！よろしくお願いします！", task.Reviewer)
    return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// PostLeaveMessage はチャンネル退出前にメッセージを投稿します
func PostLeaveMessage(channelID string) error {
    message := "さようなら！また必要になったら呼んでくださいね！👋"
    body := map[string]interface{}{
        "channel": channelID,
        "text":    message,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return fmt.Errorf("slack API response parse error: %v", err)
    }
    return nil
}

// LeaveSlackChannel はボットをチャンネルから退出させます
func LeaveSlackChannel(channelID string) error {
    body := map[string]interface{}{
        "channel": channelID,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/conversations.leave", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return fmt.Errorf("slack API response parse error: %v", err)
    }

    if !result.OK && result.Error != "not_in_channel" {
        return fmt.Errorf("slack error: %s", result.Error)
    }
    return nil
}

// PostJoinMessage はチャンネル参加時にメッセージを投稿します
func PostJoinMessage(channelID string) error {
    message := "こんにちは！レビュー通知botです。`/slack-review-notify help`で使い方を確認できます！"
    body := map[string]interface{}{
        "channel": channelID,
        "text":    message,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return fmt.Errorf("slack API response parse error: %v", err)
    }
    return nil
}

// PostEphemeralMessage は特定のユーザーにのみ見えるメッセージを投稿します
func PostEphemeralMessage(channelID, userID, message string) error {
    body := map[string]interface{}{
        "channel": channelID,
        "user":    userID,
        "text":    message,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postEphemeral", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return fmt.Errorf("slack API response parse error: %v", err)
    }
    return nil
}

// SendReminderPausedMessage はリマインダーが一時停止されたことを通知します
func SendReminderPausedMessage(task models.ReviewTask, duration string) error {
    var message string
    switch duration {
    case "1h":
        message = "🔔 リマインダーを1時間停止しました"
    case "2h":
        message = "🔔 リマインダーを2時間停止しました"
    case "4h":
        message = "🔔 リマインダーを4時間停止しました"
    case "today":
        message = "🔔 リマインダーを24時間停止しました"
    case "stop":
        message = "🔔 レビュー担当者が決まるまでリマインダーを停止しました"
    default:
        message = "🔔 リマインダーを一時停止しました"
    }
    return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// SendReviewerReminderMessage はレビュー担当者にリマインダーを送信します
func SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	message := fmt.Sprintf("⏰ <@%s> さん、レビューをお願いします！\n<%s|%s>", task.Reviewer, task.PRURL, task.Title)
	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// SendReminderMessage はレビュー待ちのPRにリマインダーを送信します
func SendReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	message := fmt.Sprintf("⏰ レビューをお願いします！\n<%s|%s>", task.PRURL, task.Title)
	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// IsChannelRelatedError はエラーがチャンネル関連のエラーかどうかを判定します
func IsChannelRelatedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "channel_not_found") ||
		strings.Contains(errStr, "not_in_channel") ||
		strings.Contains(errStr, "is_archived") ||
		strings.Contains(errStr, "channel is archived") ||
		strings.Contains(errStr, "not accessible")
}
