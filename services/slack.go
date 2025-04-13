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

func SendSlackMessage(prURL, title, channel string) (string, string, error) {
    blocks := []Block{
        {
            Type: "section",
            Text: &TextObject{
                Type: "mrkdwn",
                Text: fmt.Sprintf("<@U08MRE10GS2> *🔍 新しいレビュー対象のPRがあります！*\n\n*タイトル*: %s\n*リンク*: <%s>", title, prURL),
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
    status := "❓未割り当て"
    if task.Status == "pending" && task.Reviewer != "" {
        status = fmt.Sprintf("✅ <@%s> さんがレビュー担当です！", task.Reviewer)
    } else if task.Status == "watching" {
        status = fmt.Sprintf("👀 <@%s> さんが見てるところです", task.Reviewer)
    }

    // まず元のメッセージを更新
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

    // スレッドにメッセージを投稿
    if task.Status == "pending" && task.Reviewer != "" {
        postToThread(channel, ts, fmt.Sprintf("<@%s> さんがレビュー担当になりました", task.Reviewer))
    } else if task.Status == "watching" {
        postToThread(channel, ts, fmt.Sprintf("<@%s> さんが確認中です（2時間）", task.Reviewer))
    }

    return nil
}

// スレッドにメッセージを投稿する関数
func postToThread(channel, ts, message string) error {
    body := map[string]interface{}{
        "channel": channel,
        "thread_ts": ts,  // これがスレッド投稿の重要なパラメータ
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

    // レスポンスをログに記録
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("🧵 スレッド投稿レスポンス:", string(bodyBytes))

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
        err := SendReminderMessage(task)
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

// リマインダーメッセージをボタン付きで送信する関数（未割り当てタスク用）
func SendReminderMessage(task models.ReviewTask) error {
    // リマインダーメッセージ本文
    message := fmt.Sprintf("<@U08MRE10GS2> PRのレビューが必要です。対応できる方はメインメッセージのボタンから対応してください！\n*タイトル*: %s\n*リンク*: <%s>", 
        task.Title, task.PRURL)
    
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
                        "text": "ちょっと待って！",
                    },
                    "action_id": "pause_reminder",
                    "value": task.ID, // タスクのIDをvalueに設定
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

// CheckPendingTasks とCheckInReviewTasks 関数を修正して
// ReminderPausedUntil を考慮するようにします
func CheckPendingTasks(db *gorm.DB) {
    var tasks []models.ReviewTask
    
    // "pending" 状態でレビュアーがまだ割り当てられていないタスクを検索
    result := db.Where("status = ? AND reviewer = ?", "pending", "").Find(&tasks)
    
    if result.Error != nil {
        log.Printf("レビュー待ちタスクの確認中にエラーが発生しました: %v", result.Error)
        return
    }
    
    now := time.Now()
    tenSecondsAgo := now.Add(-10 * time.Second)
    
    for _, task := range tasks {
        // リマインダー一時停止中かチェック
        if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
            continue // 一時停止中なのでスキップ
        }
        
        // 10秒ごとにリマインダーを送信（最終更新から10秒経過しているか確認）
        if task.UpdatedAt.Before(tenSecondsAgo) {
            err := SendReminderMessage(task)
            if err != nil {
                log.Printf("リマインダー送信失敗 (Task ID: %s): %v", task.ID, err)
                continue
            }
            
            // 更新時間を記録
            task.UpdatedAt = now
            db.Save(&task)
            
            log.Printf("✅ レビュー待ちリマインダーを送信しました: %s (%s)", task.Title, task.ID)
        }
    }
}

func CheckInReviewTasks(db *gorm.DB) {
    var tasks []models.ReviewTask
    
    // "in_review" 状態でレビュアーが割り当てられているタスクを検索
    result := db.Where("status = ? AND reviewer != ?", "in_review", "").Find(&tasks)
    
    if result.Error != nil {
        log.Printf("レビュー中タスクの確認中にエラーが発生しました: %v", result.Error)
        return
    }
    
    now := time.Now()
    tenSecondsAgo := now.Add(-10 * time.Second)
    
    for _, task := range tasks {
        // リマインダー一時停止中かチェック
        if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
            continue // 一時停止中なのでスキップ
        }
        
        // 10秒ごとにリマインダーを送信（最終更新から10秒経過しているか確認）
        if task.UpdatedAt.Before(tenSecondsAgo) {
            err := SendReviewerReminderMessage(task)
            if err != nil {
                log.Printf("レビュアーリマインダー送信失敗 (Task ID: %s): %v", task.ID, err)
                continue
            }
            
            // 更新時間を記録
            task.UpdatedAt = now
            db.Save(&task)
            
            log.Printf("✅ レビュアーリマインダーを送信しました: %s (%s)", task.Title, task.ID)
        }
    }
}

// レビュアー向けのリマインダーメッセージをボタン付きで送信
func SendReviewerReminderMessage(task models.ReviewTask) error {
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
                    "type": "button",
                    "text": map[string]string{
                        "type": "plain_text",
                        "text": "ちょっと待って！",
                    },
                    "action_id": "pause_reminder",
                    "value": task.ID, // タスクIDをvalueに設定
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
func SendReminderPausedMessage(task models.ReviewTask) error {
    message := "はい！30秒間リマインドをストップします！"
    return postToThread(task.SlackChannel, task.SlackTS, message)
}
