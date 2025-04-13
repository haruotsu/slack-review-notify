package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

func SendSlackMessage(prURL, title, channel, mentionID string) (string, string, error) {
    blocks := []Block{
        {
            Type: "section",
            Text: &TextObject{
                Type: "mrkdwn",
                Text: fmt.Sprintf("<@%s> *🔍 新しいレビュー対象のPRがあります！*\n\n*タイトル*: %s\n*リンク*: <%s>", mentionID, title, prURL),
            },
        },
        {
            Type: "actions",
            Elements: []Button{
                {
                    Type: "button",
                    Text: TextObject{
                        Type: "plain_text",
                        Text: "レビューします！",
                    },
                    ActionID: "review_take",
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

	// ✅ レスポンスボディを一度全部読む
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("🔍 Slackレスポンス:", string(bodyBytes))

    // ✅ ここで読み取ったbodyをJSONパース
    var slackResp SlackPostResponse
    if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
        return "", "", err
    }

    if !slackResp.OK {
        return "", "", fmt.Errorf("Slack error: %s", slackResp.Error)
    }


	return slackResp.Ts, slackResp.Channel, nil
}

func UpdateSlackMessage(channel, ts string, task models.ReviewTask) error {
    var status string
    
    // ここでステータスと表示内容の決定ロジックを修正
    if task.Status == "in_review" && task.Reviewer != "" {
        // レビュー担当者が割り当てられている場合
        status = fmt.Sprintf("✅ <@%s> さんがレビュー担当です！", task.Reviewer)
    } else if task.Status == "watching" && task.Reviewer != "" {
        // 「今見てる！」状態
        status = fmt.Sprintf("👀 <@%s> さんが見てるところです", task.Reviewer)
    } else if task.Status == "paused" {
        // 通知が完全に停止されている状態
        status = "⏸️ リマインダーは停止中です"
    } else {
        // その他のケース（未割り当てなど）
        status = "❓未割り当て"
    }
    
    // デバッグログ追加
    log.Printf("UpdateSlackMessage: status=%s, reviewer=%s, taskStatus=%s", 
        status, task.Reviewer, task.Status)
    
    body := map[string]interface{}{
        "channel": channel,
        "ts":      ts,
        "blocks": []map[string]interface{}{
            {
                "type": "section",
                "text": map[string]string{
                    "type": "mrkdwn",
                    "text": fmt.Sprintf("*%s*\n\n*タイトル*: %s\n*リンク*: <%s>", status, task.Title, task.PRURL),
                },
            },
        },
    }

    jsonData, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", "https://slack.com/api/chat.update", bytes.NewBuffer(jsonData))
    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // レスポンス内容をデバッグログに出力
    bodyBytes, _ := io.ReadAll(resp.Body)
    log.Printf("Slack更新レスポンス: %s", string(bodyBytes))
    
    return nil
}

// スレッドにメッセージを投稿する関数
func postToThread(channel, ts, message string) error {
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

    // レスポンスをパースして詳細なエラーを取得
    var result struct {
        OK    bool   `json:"ok"`
        Error string `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    json.Unmarshal(bodyBytes, &result)
    
    log.Printf("🧵 スレッド投稿レスポンス: %s", string(bodyBytes))
    
    if !result.OK {
        return fmt.Errorf("slack error: %s", result.Error)
    }

    return nil
}

// CheckWatchingTasks は期限切れのウォッチングタスクをチェックして通知を送ります
func CheckWatchingTasks(db *gorm.DB) {
    var tasks []models.ReviewTask
    
    // "watching" 状態で、WatchingUntilが過去の時間であるか、
    // "reminded" 状態で最終更新から10秒以上経過しているタスクを検索
    now := time.Now()
    tenSecondsAgo := now.Add(-10 * time.Second)
    
    result := db.Where(
        "(status = ? AND watching_until < ?) OR (status = ? AND updated_at < ?)", 
        "watching", now, 
        "reminded", tenSecondsAgo,
    ).Find(&tasks)
    
    if result.Error != nil {
        log.Printf("ウォッチングタスクの確認中にエラーが発生しました: %v", result.Error)
        return
    }
    
    for _, task := range tasks {
        // リマインダーを送信
        err := SendReminderMessage(db, task)
        if err != nil {
            log.Printf("リマインダー送信失敗 (Task ID: %s): %v", task.ID, err)
            continue
        }
        
        // タスクのステータスを更新（リマインダー済みのステータスに）
        task.Status = "reminded"
        task.UpdatedAt = now  // 更新時間を記録
        db.Save(&task)
        
        log.Printf("✅ リマインダーを送信しました: %s (%s)", task.Title, task.ID)
    }
}

// リマインダーメッセージを送信する関数
func SendReminderMessage(db *gorm.DB, task models.ReviewTask) error {
    // チャンネルがアーカイブされているか確認
    isArchived, err := IsChannelArchived(task.SlackChannel)
    if err != nil {
        log.Printf("チャンネル状態確認エラー（チャンネル: %s）: %v", task.SlackChannel, err)
        
        // Slack APIエラーの場合、エラーの種類を確認
        if strings.Contains(err.Error(), "not_in_channel") || 
           strings.Contains(err.Error(), "channel_not_found") {
            log.Printf("チャンネルにボットが参加していないか、チャンネルが存在しません: %s", task.SlackChannel)
            
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
                log.Printf("✅ チャンネル %s の設定を非アクティブにしました", task.SlackChannel)
            }
            
            return fmt.Errorf("channel is archived or not accessible: %s", task.SlackChannel)
        }
    }
    
    if isArchived {
        log.Printf("チャンネル %s はアーカイブされています", task.SlackChannel)
        
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
            log.Printf("✅ チャンネル %s の設定を非アクティブにしました", task.SlackChannel)
        }
        
        return fmt.Errorf("channel is archived: %s", task.SlackChannel)
    }
    
    // リマインダーメッセージ本文
    message := fmt.Sprintf("<@U08MRE10GS2> PRのレビューが必要です。対応できる方はメインメッセージのボタンから対応してください！\n*タイトル*: %s\n*リンク*: <%s>", 
        task.Title, task.PRURL)
    
    // デバッグログを追加
    log.Printf("リマインダー送信時のタスクID: %s", task.ID)
    
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
                                "text": "20秒間停止",
                            },
                            "value": fmt.Sprintf("%s:20s", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "30秒間停止",
                            },
                            "value": fmt.Sprintf("%s:30s", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "1分間停止",
                            },
                            "value": fmt.Sprintf("%s:1m", task.ID),
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
    fmt.Println("🧵 リマインダー投稿レスポンス:", string(bodyBytes))
    
    return nil
}

// レビュー担当者が決まった時のメッセージ
func SendReviewerAssignedMessage(task models.ReviewTask) error {
    message := fmt.Sprintf("✅ <@%s> さんがレビュー担当になりました！", task.Reviewer)
    return postToThread(task.SlackChannel, task.SlackTS, message)
}

// レビュアー向けのリマインダーメッセージも同様に修正
func SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
    // チャンネルがアーカイブされているか確認
    isArchived, err := IsChannelArchived(task.SlackChannel)
    if err != nil {
        log.Printf("チャンネル状態確認エラー（チャンネル: %s）: %v", task.SlackChannel, err)
        
        // Slack APIエラーの場合、エラーの種類を確認
        if strings.Contains(err.Error(), "not_in_channel") || 
           strings.Contains(err.Error(), "channel_not_found") {
            log.Printf("チャンネルにボットが参加していないか、チャンネルが存在しません: %s", task.SlackChannel)
            
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
                log.Printf("✅ チャンネル %s の設定を非アクティブにしました", task.SlackChannel)
            }
            
            return fmt.Errorf("channel is archived or not accessible: %s", task.SlackChannel)
        }
    }
    
    if isArchived {
        log.Printf("チャンネル %s はアーカイブされています", task.SlackChannel)
        
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
            log.Printf("✅ チャンネル %s の設定を非アクティブにしました", task.SlackChannel)
        }
        
        return fmt.Errorf("channel is archived: %s", task.SlackChannel)
    }
    
    message := fmt.Sprintf("<@%s> レビューの進捗はいかがですか？まだ完了していない場合は対応をお願いします！", task.Reviewer)
    
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
                                "text": "20秒間停止",
                            },
                            "value": fmt.Sprintf("%s:20s", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "30秒間停止",
                            },
                            "value": fmt.Sprintf("%s:30s", task.ID),
                        },
                        {
                            "text": map[string]string{
                                "type": "plain_text",
                                "text": "1分間停止",
                            },
                            "value": fmt.Sprintf("%s:1m", task.ID),
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
    case "20s":
        message = "はい！20秒間リマインドをストップします！"
    case "30s":
        message = "はい！30秒間リマインドをストップします！"
    case "1m":
        message = "はい！1分間リマインドをストップします！"
    case "today":
        message = "今日はもうリマインドしません。明日また通知します！"
    case "stop":
        message = "リマインダーを完全に停止しました。レビュー担当者が決まるまで通知しません。"
    default:
        message = "リマインドをストップします！"
    }
    
    return postToThread(task.SlackChannel, task.SlackTS, message)
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
            // その他のチャンネル情報
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
        // チャンネルIDを収集
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

// チャンネルのボットの参加状態を確認
func IsBotInChannel(channelID string) (bool, error) {
    url := fmt.Sprintf("https://slack.com/api/conversations.members?channel=%s", channelID)
    
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
        OK      bool     `json:"ok"`
        Members []string `json:"members"`
        Error   string   `json:"error"`
    }
    
    bodyBytes, _ := io.ReadAll(resp.Body)
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        return false, err
    }
    
    if !result.OK {
        return false, fmt.Errorf("slack error: %s", result.Error)
    }
    
    botUserID := os.Getenv("SLACK_BOT_USER_ID")
    if botUserID == "" {
        return false, fmt.Errorf("SLACK_BOT_USER_ID is not set")
    }
    
    for _, member := range result.Members {
        if member == botUserID {
            return true, nil
        }
    }
    
    return false, nil
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
