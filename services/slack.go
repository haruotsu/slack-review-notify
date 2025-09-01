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

	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

// テストモードかどうかを示すフラグ
var IsTestMode bool

type SlackMessage struct {
	Channel string  `json:"channel"`
	Blocks  []Block `json:"blocks"`
}

type Block struct {
	Type     string      `json:"type"`
	Text     *TextObject `json:"text,omitempty"`
	Elements []Button    `json:"elements,omitempty"`
}

type TextObject struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Button struct {
	Type     string     `json:"type"`
	Text     TextObject `json:"text"`
	ActionID string     `json:"action_id"`
	Style    string     `json:"style,omitempty"`
}

type SlackPostResponse struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel"`
	Ts      string `json:"ts"`
	Error   string `json:"error,omitempty"`
}

func ValidateSlackRequest(r *http.Request, body []byte) bool {
	// テストモードの場合は常に検証成功とする
	if IsTestMode {
		return true
	}

	slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if slackSigningSecret == "" {
		log.Println("SLACK_SIGNING_SECRET is not set")
		return false
	}

	sv, err := slack.NewSecretsVerifier(r.Header, slackSigningSecret)
	if err != nil {
		log.Printf("Failed to create secrets verifier: %v", err)
		return false
	}

	// bodyをVerifierに書き込む
	if _, err := sv.Write(body); err != nil {
		log.Printf("Failed to write body to verifier: %v", err)
		return false
	}

	// 署名を検証
	if err := sv.Ensure(); err != nil {
		log.Printf("Invalid signature: %v", err)
		return false
	}

	return true
}

// メンション先ユーザーIDをランダムに選択する関数
func SelectRandomReviewer(db *gorm.DB, channelID string, labelName string) string {
	var config models.ChannelConfig

	// チャンネルとラベルの設定を取得
	if err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error; err != nil {
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

// SendSlackMessageOffHours は営業時間外用のメンション抜きメッセージを送信する
func SendSlackMessageOffHours(prURL, title, channel string) (string, string, error) {
	message := fmt.Sprintf("📝 *レビュー対象のPRが登録されました*\n\n*PRタイトル*: %s\n*URL*: <%s>\n\n (レビューのメンションは翌営業日の朝（10時）にお送りします)", title, prURL)
	doneButton := CreateButton("レビュー完了", "review_done", "done", "primary")
	blocks := CreateMessageWithActionBlocks(message, doneButton)

	body := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
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

	var result struct {
		OK      bool   `json:"ok"`
		TS      string `json:"ts"`
		Channel string `json:"channel"`
		Error   string `json:"error"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", "", fmt.Errorf("slack API response parse error: %v", err)
	}

	if !result.OK {
		return "", "", fmt.Errorf("slack error: %s", result.Error)
	}

	return result.TS, result.Channel, nil
}

// PostBusinessHoursNotificationToThread は営業時間になったときにスレッドにメンション付き通知を送信する
func PostBusinessHoursNotificationToThread(task models.ReviewTask, mentionID string) error {
	// ユーザーIDまたはチームIDのメンション形式を決定
	var mentionText string
	if strings.HasPrefix(mentionID, "subteam^") || strings.HasPrefix(mentionID, "S") {
		// チームIDの場合はsubteam形式で表示
		mentionText = fmt.Sprintf("<!subteam^%s>", mentionID)
	} else {
		// ユーザーIDの場合は通常のメンション形式
		mentionText = fmt.Sprintf("<@%s>", mentionID)
	}
	
	// レビュワーが設定されている場合は追加
	var reviewerText string
	if task.Reviewer != "" {
		reviewerText = fmt.Sprintf("\n\n🎯 *レビュワー*: <@%s> さん、よろしくお願いします！", task.Reviewer)
	}

	message := fmt.Sprintf("🌅 *おはようございます！* %s\n\n📋 こちらのPRのレビューをお願いします。%s", mentionText, reviewerText)
	
	blocks := CreateMessageBlocks(message)

	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
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

func SendSlackMessage(prURL, title, channel, mentionID string) (string, string, error) {
	// ユーザーIDまたはチームIDのメンション形式を決定
	var mentionText string
	if strings.HasPrefix(mentionID, "subteam^") || strings.HasPrefix(mentionID, "S") {
		// チームIDの場合はsubteam形式で表示
		mentionText = fmt.Sprintf("<!subteam^%s>", mentionID)
	} else {
		// ユーザーIDの場合は通常のメンション形式
		mentionText = fmt.Sprintf("<@%s>", mentionID)
	}

	message := fmt.Sprintf("%s *レビュー対象のPRがあります！*\n\n*PRタイトル*: %s\n*URL*: <%s>", mentionText, title, prURL)
	doneButton := CreateButton("レビュー完了", "review_done", "done", "primary")
	blocks := CreateMessageWithActionBlocks(message, doneButton)

	body := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
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
		"channel":   channel,
		"thread_ts": ts,
		"text":      message,
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

// スレッドにボタン付きメッセージを投稿する関数
func PostToThreadWithButtons(channel, ts, message string, taskID string) error {
	pauseButton := CreateButton("リマインドを一時停止", "pause_reminder_thread", taskID, "danger")
	blocks := CreateMessageWithActionBlocks(message, pauseButton)

	body := map[string]interface{}{
		"channel":   channel,
		"thread_ts": ts,
		"blocks":    blocks,
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

	pauseSelect := CreateAllOptionsPauseReminderSelect(task.ID, "pause_reminder", "リマインダーを停止")
	blocks := CreateMessageWithActionBlocks(message, pauseSelect)

	// スレッドにボタン付きメッセージを投稿
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
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
		message = "今日はもうリマインドしません。翌営業日の朝に再開します！"
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
			ID         string `json:"id"`
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
	message := fmt.Sprintf("自動でレビュワーが割り当てられました: <@%s> レビューをお願いします！", task.Reviewer)

	changeButton := CreateChangeReviewerButton(task.ID)
	pauseSelect := CreateAllOptionsPauseReminderSelect(task.ID, "pause_reminder_initial", "リマインダーを停止")
	blocks := CreateMessageWithActionsBlocks(message, changeButton, pauseSelect)

	// スレッドにボタン付きメッセージを投稿
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
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

// 営業時間外かどうかを判定する関数
// 営業時間：平日の10:00〜18:59、それ以外は営業時間外
func IsOutsideBusinessHours(t time.Time) bool {
	// JST タイムゾーンを取得
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		// フォールバック：元のタイムゾーンを使用
		jst = t.Location()
	}

	// 時刻をJSTに変換
	timeInJST := t.In(jst)
	
	// 土日は営業時間外
	weekday := timeInJST.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true
	}
	
	// 平日の営業時間は10:00〜18:59
	hour := timeInJST.Hour()
	if hour < 10 || hour >= 19 {
		return true
	}
	
	return false
}

// 翌営業日の朝（10:00）の時間を取得する関数
func GetNextBusinessDayMorning() time.Time {
	return GetNextBusinessDayMorningWithTime(time.Now())
}

// 指定された時刻から翌営業日の朝（10:00）の時間を取得する関数
func GetNextBusinessDayMorningWithTime(now time.Time) time.Time {
	// JST タイムゾーンを取得
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		// フォールバック：現在のタイムゾーンを使用
		jst = now.Location()
	}

	// 現在時刻をJSTに変換
	nowInJST := now.In(jst)
	
	// 今日の10:00（JST）を作成
	todayMorning := time.Date(nowInJST.Year(), nowInJST.Month(), nowInJST.Day(), 10, 0, 0, 0, jst)

	// 現在の曜日と時刻を確認（JST基準）
	weekday := nowInJST.Weekday()
	
	// 結果を格納する変数
	var nextBusinessDayMorning time.Time

	switch weekday {
	case time.Monday, time.Tuesday, time.Wednesday, time.Thursday:
		// 月〜木の場合
		if nowInJST.Before(todayMorning) {
			// 10:00前なら今日の10:00
			nextBusinessDayMorning = todayMorning
		} else {
			// 10:00以降なら翌日の10:00
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
		}
	case time.Friday:
		// 金曜日の場合
		if nowInJST.Before(todayMorning) {
			// 10:00前なら今日の10:00
			nextBusinessDayMorning = todayMorning
		} else {
			// 10:00以降なら月曜日の10:00（3日後）
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 3)
		}
	case time.Saturday:
		// 土曜日の場合、月曜日の10:00（2日後）
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 2)
	case time.Sunday:
		// 日曜日の場合、月曜日の10:00（1日後）
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
	}

	return nextBusinessDayMorning
}

// SendOutOfHoursReminderMessage は営業時間外のリマインドメッセージを送信する
func SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	message := fmt.Sprintf("<@%s> レビューしてくれたら嬉しいです...👀\n\n営業時間外のため、次回のリマインドは翌営業日に送信します。", task.Reviewer)

	pauseSelect := CreateStopOnlyPauseReminderSelect(task.ID, "pause_reminder", "リマインダーを停止")
	blocks := CreateMessageWithActionBlocks(message, pauseSelect)

	// スレッドにボタン付きメッセージを投稿
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
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

// UpdateSlackMessageForCompletedTask はタスクが完了したことを示すようにSlackメッセージを更新する
func UpdateSlackMessageForCompletedTask(task models.ReviewTask) error {
	if IsTestMode {
		log.Printf("test mode: would update slack message for completed task: %s", task.ID)
		return nil
	}

	message := fmt.Sprintf("✅ *%s*\n🔗 %s\n\n*レビュー完了*: このPRのラベルが外れたため、レビュータスクを終了しました。", task.Title, task.PRURL)
	blocks := CreateMessageBlocks(message)

	// メッセージ更新API呼び出し
	body := map[string]interface{}{
		"channel": task.SlackChannel,
		"ts":      task.SlackTS,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://slack.com/api/chat.update", bytes.NewBuffer(jsonData))
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack API error: %s", string(body))
	}

	log.Printf("slack message updated for completed task: %s", task.ID)
	return nil
}

// レビュー完了の自動通知を送信する関数
func SendReviewCompletedAutoNotification(task models.ReviewTask, reviewerLogin string, reviewState string) error {
	var message string
	var emoji string

	switch reviewState {
	case "approved":
		emoji = "✅"
		message = fmt.Sprintf("%s %sさんがレビューを承認しました！感謝！👏", emoji, reviewerLogin)
	case "changes_requested":
		emoji = "🔄"
		message = fmt.Sprintf("%s %sさんが変更を要求しました 感謝！👏", emoji, reviewerLogin)
	case "commented":
		emoji = "💬"
		message = fmt.Sprintf("%s %sさんがレビューコメントを残しました 感謝！👏", emoji, reviewerLogin)
	default:
		emoji = "👀"
		message = fmt.Sprintf("%s %sさんがレビューしました 感謝！👏", emoji, reviewerLogin)
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}
