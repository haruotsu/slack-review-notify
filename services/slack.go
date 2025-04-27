package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"slack-review-notify/models"
	"strings"
	"time"

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

// メンション先ユーザーIDをランダムに選択する関数
func SelectRandomReviewer(db *gorm.DB, channelID string) string {
	var config models.ChannelConfig
	
	// チャンネルの設定を取得
	if err := db.Where("slack_channel_id = ?", channelID).First(&config).Error; err != nil {
		log.Printf("failed to get channel config: %v", err)
		return ""
	}
	
	// レビュワーリストが空の場合はデフォルトのメンション先を返す
	if config.ReviewerList == "" {
		return config.DefaultMentionID
	}
	
	// レビュワーリストからランダムに選択
	reviewers := strings.Split(config.ReviewerList, ",")
	
	// 空の要素を削除
	var validReviewers []string
	for _, r := range reviewers {
		if trimmed := strings.TrimSpace(r); trimmed != "" {
			validReviewers = append(validReviewers, trimmed)
		}
	}
	
	if len(validReviewers) == 0 {
		return config.DefaultMentionID
	}
	
	// 乱数生成のシードを設定
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	
	// ランダムなインデックスを生成
	randomIndex := r.Intn(len(validReviewers))
	
	return validReviewers[randomIndex]
}

func SendSlackMessage(prURL, title, channel, mentionID string) (string, string, error) {
    // ユーザーIDの先頭に@を付けてメンション形式に変換
    blocks := []Block{
        {
            Type: "section",
            Text: &TextObject{
                Type: "mrkdwn",
                Text: fmt.Sprintf("<@%s> *レビュー対象のPRがあります！*\n\n*PRタイトル*: %s\n*URL*: <%s>", mentionID, title, prURL),
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
                    Style: "primary",
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
    
    log.Printf("slack thread post response: %s", string(bodyBytes))
    
    if !result.OK {
        return fmt.Errorf("slack error: %s", result.Error)
    }

    return nil
}

// リマインダーメッセージを送信する関数
func SendReminderMessage(db *gorm.DB, task models.ReviewTask) error {
    // チャンネルがアーカイブされているか確認
    isArchived, err := IsChannelArchived(task.SlackChannel)
    if err != nil {
        log.Printf("channel status check error (channel: %s): %v", task.SlackChannel, err)
        
        // Slack APIエラーの場合、エラーの種類を確認
        if strings.Contains(err.Error(), "not_in_channel") || 
           strings.Contains(err.Error(), "channel_not_found") {
            log.Printf("bot is not in channel or channel not found: %s", task.SlackChannel)
            
            // タスクを無効化
            task.Status = "archived"
            task.UpdatedAt = time.Now()
            db.Save(&task)
            
            // チャンネル設定も無効化
            var config models.ChannelConfig
            if result := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config); result.Error == nil {
                config.IsActive = false
                config.UpdatedAt = time.Now()
                db.Save(&config)
                log.Printf("channel %s config is deactivated", task.SlackChannel)
            }
            
            return fmt.Errorf("channel is archived or not accessible: %s", task.SlackChannel)
        }
    }
    
    if isArchived {
        log.Printf("channel %s is archived", task.SlackChannel)
        
        // タスクを無効化
        task.Status = "archived"
        task.UpdatedAt = time.Now()
        db.Save(&task)
        
        // チャンネル設定も無効化
        var config models.ChannelConfig
        if result := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config); result.Error == nil {
            config.IsActive = false
            config.UpdatedAt = time.Now()
            db.Save(&config)
            log.Printf("channel %s config is deactivated", task.SlackChannel)
        }
        
        return fmt.Errorf("channel is archived: %s", task.SlackChannel)
    }
    
    // リマインダーメッセージ本文
    message := fmt.Sprintf("PRのレビューが必要です。素早いレビューで速くバリューを届けましょう！対応できる方はメインメッセージのボタンから！\n*タイトル*: %s\n*リンク*: <%s>", 
        task.Title, task.PRURL)
    
    // デバッグログを追加
    log.Printf("reminder task id: %s", task.ID)
    
    // ボタン付きのメッセージブロックを作成
    blocks := []map[string]interface{}{
        {
            "type": "section",
            "text": map[string]string{
                "type": "mrkdwn",
                "text": message,
            },
        },
        {
            "type": "actions",
            "elements": []map[string]interface{}{
                {
                    "type": "static_select",
                    "placeholder": map[string]string{
                        "type": "plain_text",
                        "text": "リマインダーを停止...",
                    },
                    "action_id": "pause_reminder",
                    "options": []map[string]interface{}{
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "1時間停止",
                            },
                            "value": fmt.Sprintf("%s:1h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "2時間停止",
                            },
                            "value": fmt.Sprintf("%s:2h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "4時間停止",
                            },
                            "value": fmt.Sprintf("%s:4h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "今日は通知しない",
                            },
                            "value": fmt.Sprintf("%s:today", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "リマインダーを完全に停止",
                            },
                            "value": fmt.Sprintf("%s:stop", task.ID),
                        },
                    },
                },
            },
        },
    }
    
    // スレッドにボタン付きメッセージを投稿
    body := map[string]interface{}{
        "channel": task.SlackChannel,
        "thread_ts": task.SlackTS,
        "blocks": blocks,
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
    
    // レスポンスをログに記録
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("slack reminder post response:", string(bodyBytes))
    
    return nil
}

// レビュアー向けのリマインダーメッセージ
func SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
    // チャンネルがアーカイブされているか確認
    isArchived, err := IsChannelArchived(task.SlackChannel)
    if err != nil {
        log.Printf("channel status check error (channel: %s): %v", task.SlackChannel, err)
        
        // Slack APIエラーの場合、エラーの種類を確認
        if strings.Contains(err.Error(), "not_in_channel") || 
           strings.Contains(err.Error(), "channel_not_found") {
            log.Printf("bot is not in channel or channel not found: %s", task.SlackChannel)
            
            // タスクを無効化
            task.Status = "archived"
            task.UpdatedAt = time.Now()
            db.Save(&task)
            
            // チャンネル設定も無効化
            var config models.ChannelConfig
            if result := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config); result.Error == nil {
                config.IsActive = false
                config.UpdatedAt = time.Now()
                db.Save(&config)
                log.Printf("channel %s config is deactivated", task.SlackChannel)
            }
            
            return fmt.Errorf("channel is archived or not accessible: %s", task.SlackChannel)
        }
    }
    
    if isArchived {
        log.Printf("channel %s is archived", task.SlackChannel)
        
        // タスクを無効化
        task.Status = "archived"
        task.UpdatedAt = time.Now()
        db.Save(&task)
        
        // チャンネル設定も無効化
        var config models.ChannelConfig
        if result := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config); result.Error == nil {
            config.IsActive = false
            config.UpdatedAt = time.Now()
            db.Save(&config)
            log.Printf("channel %s config is deactivated", task.SlackChannel)
        }
        
        return fmt.Errorf("channel is archived: %s", task.SlackChannel)
    }
    
    message := fmt.Sprintf("<@%s> レビューしてくれたら嬉しいです...👀", task.Reviewer)
    
    // ボタン付きのメッセージブロックを作成
    blocks := []map[string]interface{}{
        {
            "type": "section",
            "text": map[string]string{
                "type": "mrkdwn",
                "text": message,
            },
        },
        {
            "type": "actions",
            "elements": []map[string]interface{}{
                {
                    "type": "static_select",
                    "placeholder": map[string]string{
                        "type": "plain_text",
                        "text": "リマインダーを停止...",
                    },
                    "action_id": "pause_reminder",
                    "options": []map[string]interface{}{
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "1時間停止",
                            },
                            "value": fmt.Sprintf("%s:1h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "2時間停止",
                            },
                            "value": fmt.Sprintf("%s:2h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "4時間停止",
                            },
                            "value": fmt.Sprintf("%s:4h", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "今日は通知しない",
                            },
                            "value": fmt.Sprintf("%s:today", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "リマインダーを完全に停止",
                            },
                            "value": fmt.Sprintf("%s:stop", task.ID),
                        },
                    },
                },
            },
        },
    }
    
    // スレッドにボタン付きメッセージを投稿
    body := map[string]interface{}{
        "channel": task.SlackChannel,
        "thread_ts": task.SlackTS,
        "blocks": blocks,
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
    
    return nil
}

// リマインダーを一時停止したことを通知する関数
func SendReminderPausedMessage(task models.ReviewTask, duration string) error {
    var message string
    
    switch duration {
    case "1h":
        message = "はい！1時間リマインドをストップします！"
    case "2h":
        message = "はい！2時間リマインドをストップします！"
    case "4h":
        message = "はい！4時間リマインドをストップします！"
    case "today":
        message = "今日はもうリマインドしません。24時間後に再開します！"
    case "stop":
        message = "リマインダーを完全に停止しました。レビュー担当者が決まるまで通知しません。"
    default:
        message = "リマインドをストップします！"
    }
    
    return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// ボットが参加しているチャンネルのリストを取得
func GetBotChannels() ([]string, error) {
    url := "https://slack.com/api/conversations.list?types=public_channel,private_channel"
    
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result struct {
        OK       bool `json:"ok"`
        Channels []struct {
            ID   string `json:"id"`
            Name string `json:"name"`
        } `json:"channels"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    if !result.OK {
        return nil, fmt.Errorf("failed to get channels")
    }
    
    channelIDs := make([]string, 0, len(result.Channels))
    for _, ch := range result.Channels {
        channelIDs = append(channelIDs, ch.ID)
    }
    
    return channelIDs, nil
}

// SlackのAPIエラーが「チャンネル関連のエラー」かどうかを判定
func IsChannelRelatedError(err error) bool {
    if err == nil {
        return false
    }
    
    errorStr := err.Error()
    return strings.Contains(errorStr, "not_in_channel") || 
           strings.Contains(errorStr, "channel_not_found") || 
           strings.Contains(errorStr, "is_archived") || 
           strings.Contains(errorStr, "missing_scope") ||
           strings.Contains(errorStr, "channel_not_found")
}

// チャンネルがアーカイブされているかどうかを確認する関数
func IsChannelArchived(channelID string) (bool, error) {
    url := fmt.Sprintf("https://slack.com/api/conversations.info?channel=%s", channelID)
    
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return false, err
    }
    
    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return false, err
    }
    defer resp.Body.Close()
    
    var result struct {
        OK      bool `json:"ok"`
        Channel struct {
            ID        string `json:"id"`
            IsArchived bool   `json:"is_archived"`
        } `json:"channel"`
        Error string `json:"error"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return false, err
    }
    
    if !result.OK {
        if result.Error == "channel_not_found" {
            // チャンネルが存在しない場合はアーカイブされていると見なす
            return true, nil
        }
        return false, fmt.Errorf("failed to get channel info: %s", result.Error)
    }
    
    return result.Channel.IsArchived, nil
}

// 自動割り当てされたレビュワーを表示し、変更ボタンを表示する関数
func PostReviewerAssignedMessageWithChangeButton(task models.ReviewTask) error {
    message := fmt.Sprintf("自動でレビュワーが割り当てられました: <@%s> さん、レビューをお願いします！", task.Reviewer)
    
    // ボタン付きのメッセージブロックを作成
    blocks := []map[string]interface{}{
        {
            "type": "section",
            "text": map[string]string{
                "type": "mrkdwn",
                "text": message,
            },
        },
        {
            "type": "actions",
            "elements": []map[string]interface{}{
                {
                    "type": "button",
                    "text": map[string]string{
                        "type": "plain_text",
                        "text": "変わってほしい！",
                    },
                    "action_id": "change_reviewer",
                    "value": task.ID,
                    "style": "danger",
                },
            },
        },
    }
    
    // スレッドにボタン付きメッセージを投稿
    body := map[string]interface{}{
        "channel": task.SlackChannel,
        "thread_ts": task.SlackTS,
        "blocks": blocks,
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
    
    return nil
}

// レビュワーが変更されたことを通知する関数
func SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error {
    message := fmt.Sprintf("レビュワーを変更しました: <@%s> → <@%s> さん、よろしくお願いします！", oldReviewerID, task.Reviewer)
    return PostToThread(task.SlackChannel, task.SlackTS, message)
}
