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

func GetSlackUserIDFromGitHub(db *gorm.DB, githubUsername string) string {
	if githubUsername == "" {
		return ""
	}

	var mapping models.UserMapping
	if err := db.Where("github_username = ?", githubUsername).First(&mapping).Error; err != nil {
		log.Printf("user mapping not found for github user: %s", githubUsername)
		return ""
	}

	return mapping.SlackUserID
}

func buildMentionText(mentionID string) string {
	if strings.HasPrefix(mentionID, "subteam^") || strings.HasPrefix(mentionID, "S") {
		return fmt.Sprintf("<!subteam^%s>", mentionID)
	}
	return fmt.Sprintf("<@%s>", mentionID)
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

// SelectRandomReviewers は指定人数のレビュワーをランダム選択する（excludeIDs を除外）
func SelectRandomReviewers(db *gorm.DB, channelID string, labelName string, count int, excludeIDs []string) []string {
	var config models.ChannelConfig

	if err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error; err != nil {
		log.Printf("failed to get channel config: %v", err)
		return []string{}
	}

	if config.ReviewerList == "" {
		return []string{config.DefaultMentionID}
	}

	reviewers := strings.Split(config.ReviewerList, ",")

	excludeSet := make(map[string]bool)
	for _, id := range excludeIDs {
		if id != "" {
			excludeSet[id] = true
		}
	}

	var candidates []string
	for _, r := range reviewers {
		if trimmed := strings.TrimSpace(r); trimmed != "" && !excludeSet[trimmed] {
			candidates = append(candidates, trimmed)
		}
	}

	if len(candidates) == 0 {
		return []string{config.DefaultMentionID}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	if count > len(candidates) {
		count = len(candidates)
	}

	return candidates[:count]
}

// GetPendingReviewers は未approveのレビュワーリストを返す
func GetPendingReviewers(task models.ReviewTask) []string {
	if task.Reviewers == "" {
		if task.Reviewer != "" {
			return []string{task.Reviewer}
		}
		return nil
	}

	approvedSet := make(map[string]bool)
	if task.ApprovedBy != "" {
		for _, id := range strings.Split(task.ApprovedBy, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				approvedSet[trimmed] = true
			}
		}
	}

	var pending []string
	for _, id := range strings.Split(task.Reviewers, ",") {
		if trimmed := strings.TrimSpace(id); trimmed != "" && !approvedSet[trimmed] {
			pending = append(pending, trimmed)
		}
	}
	return pending
}

// AddApproval は task.ApprovedBy にレビュワーを追加する（重複防止）。新規追加なら true を返す
func AddApproval(task *models.ReviewTask, slackUserID string) bool {
	if slackUserID == "" {
		return false
	}

	if task.ApprovedBy != "" {
		for _, id := range strings.Split(task.ApprovedBy, ",") {
			if strings.TrimSpace(id) == slackUserID {
				return false
			}
		}
		task.ApprovedBy = task.ApprovedBy + "," + slackUserID
	} else {
		task.ApprovedBy = slackUserID
	}
	return true
}

// IsReviewFullyApproved は必要なapprove数を満たしているか判定する
func IsReviewFullyApproved(task models.ReviewTask, requiredApprovals int) bool {
	if requiredApprovals <= 0 {
		requiredApprovals = 1
	}

	if task.ApprovedBy == "" {
		return false
	}

	count := 0
	for _, id := range strings.Split(task.ApprovedBy, ",") {
		if strings.TrimSpace(id) != "" {
			count++
		}
	}

	return count >= requiredApprovals
}

// SendSlackMessageOffHours は営業時間外用のメンション抜きメッセージを送信する
func SendSlackMessageOffHours(prURL, title, channel, creatorSlackID string) (string, string, error) {
	var message string
	if creatorSlackID != "" {
		message = fmt.Sprintf("<@%s> からのレビュー依頼が登録されました\n\n*PRタイトル*: %s\n*URL*: <%s>\n\n📝 レビューのメンションは翌営業日の朝にお送りします", creatorSlackID, title, prURL)
	} else {
		message = fmt.Sprintf("📝 *レビュー対象のPRが登録されました*\n\n*PRタイトル*: %s\n*URL*: <%s>\n\n (レビューのメンションは翌営業日の朝にお送りします)", title, prURL)
	}
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	mentionText := buildMentionText(mentionID)

	// レビュワーが設定されている場合は追加
	var reviewerText string
	if task.Reviewer != "" {
		reviewerText = fmt.Sprintf("\n\n🎯 *レビュワー*: @%s さん、よろしくお願いします！", task.Reviewer)
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
	defer func() {
		_ = resp.Body.Close()
	}()

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

func SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID string) (string, string, error) {
	mentionText := buildMentionText(mentionID)

	var message string
	if creatorSlackID != "" {
		message = fmt.Sprintf("%s <@%s> からのレビュー依頼があります\n\n*PRタイトル*: %s\n*URL*: <%s>", mentionText, creatorSlackID, title, prURL)
	} else {
		message = fmt.Sprintf("%s *レビュー対象のPRがあります！*\n\n*PRタイトル*: %s\n*URL*: <%s>", mentionText, title, prURL)
	}
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
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, _ := io.ReadAll(resp.Body)

	var slackResp SlackPostResponse
	if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
		return "", "", err
	}

	if !slackResp.OK {
		return "", "", fmt.Errorf("slack error: %s", slackResp.Error)
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

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

	// 未approveのレビュワーのみメンション
	pendingReviewers := GetPendingReviewers(task)
	var mentionParts []string
	for _, id := range pendingReviewers {
		mentionParts = append(mentionParts, fmt.Sprintf("<@%s>", id))
	}
	var mentionText string
	if len(mentionParts) > 0 {
		mentionText = strings.Join(mentionParts, " ")
	} else if task.Reviewer != "" {
		mentionText = fmt.Sprintf("<@%s>", task.Reviewer)
	}
	message := fmt.Sprintf("%s レビューしてくれたら嬉しいです...👀", mentionText)

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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	var message string
	if task.Reviewers != "" {
		var mentions []string
		for _, id := range strings.Split(task.Reviewers, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				mentions = append(mentions, fmt.Sprintf("<@%s>", trimmed))
			}
		}
		message = fmt.Sprintf("自動でレビュワーが割り当てられました: %s レビューをお願いします！", strings.Join(mentions, " "))
	} else {
		message = fmt.Sprintf("自動でレビュワーが割り当てられました: <@%s> レビューをお願いします！", task.Reviewer)
	}

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
	defer func() {
		_ = resp.Body.Close()
	}()

	return nil
}

// formatReviewerMentions は複数のレビュワーIDをSlackメンション形式に変換する
func formatReviewerMentions(reviewerIDs string) string {
	if reviewerIDs == "" {
		return ""
	}

	// スペースで分割してIDを抽出
	ids := strings.Fields(reviewerIDs)

	var mentions []string
	for _, id := range ids {
		// @記号を取り除く
		cleanID := strings.TrimPrefix(id, "@")
		if cleanID != "" {
			mentions = append(mentions, fmt.Sprintf("<@%s>", cleanID))
		}
	}

	return strings.Join(mentions, " ")
}

// レビュワーが変更されたことを通知する関数
func SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error {
	oldMentions := formatReviewerMentions(oldReviewerID)
	newMentions := formatReviewerMentions(task.Reviewer)

	message := fmt.Sprintf("レビュワーを変更しました: %s → %s さん、よろしくお願いします！", oldMentions, newMentions)
	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// 指定された時刻から翌営業日の営業開始時刻を取得する関数（営業時間設定対応）
func GetNextBusinessDayMorningWithConfig(now time.Time, config *models.ChannelConfig) time.Time {
	// タイムゾーンの設定を取得
	timezone := "Asia/Tokyo"
	if config != nil && config.Timezone != "" {
		timezone = config.Timezone
	}

	// タイムゾーンをロード
	tz, err := time.LoadLocation(timezone)
	if err != nil {
		// フォールバック：現在のタイムゾーンを使用
		tz = now.Location()
	}

	// 現在時刻を指定タイムゾーンに変換
	nowInTZ := now.In(tz)

	// 営業開始時刻を取得（デフォルト: 10:00）
	businessHourStart := "10:00"
	if config != nil && config.BusinessHoursStart != "" {
		businessHourStart = config.BusinessHoursStart
	}

	// 営業開始時刻をパース
	startHour, startMin, err := parseBusinessHoursTime(businessHourStart)
	if err != nil {
		// パースに失敗した場合は10:00をデフォルトとする
		startHour, startMin = 10, 0
	}

	// 今日の営業開始時刻を作成
	todayMorning := time.Date(nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(), startHour, startMin, 0, 0, tz)

	// 現在の曜日と時刻を確認
	weekday := nowInTZ.Weekday()

	// 結果を格納する変数
	var nextBusinessDayMorning time.Time

	switch weekday {
	case time.Monday, time.Tuesday, time.Wednesday, time.Thursday:
		// 月〜木の場合
		if nowInTZ.Before(todayMorning) {
			// 営業開始時刻前なら今日の営業開始時刻
			nextBusinessDayMorning = todayMorning
		} else {
			// 営業開始時刻以降なら翌日の営業開始時刻
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
		}
	case time.Friday:
		// 金曜日の場合
		if nowInTZ.Before(todayMorning) {
			// 営業開始時刻前なら今日の営業開始時刻
			nextBusinessDayMorning = todayMorning
		} else {
			// 営業開始時刻以降なら月曜日の営業開始時刻（3日後）
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 3)
		}
	case time.Saturday:
		// 土曜日の場合、月曜日の営業開始時刻（2日後）
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 2)
	case time.Sunday:
		// 日曜日の場合、月曜日の営業開始時刻（1日後）
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
	}

	return nextBusinessDayMorning
}

// SendOutOfHoursReminderMessage は営業時間外のリマインドメッセージを送信する
func SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	message := fmt.Sprintf("@%s レビューしてくれたら嬉しいです...👀\n\n営業時間外のため、次回のリマインドは翌営業日に送信します。", task.Reviewer)

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
	defer func() {
		_ = resp.Body.Close()
	}()

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
	defer func() {
		_ = resp.Body.Close()
	}()

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

// PostLabelRemovedNotification はラベル削除によるタスク完了をスレッドに通知する
func PostLabelRemovedNotification(task models.ReviewTask, removedLabels []string) error {
	if IsTestMode {
		log.Printf("test mode: would post label removed notification for task: %s", task.ID)
		return nil
	}

	var labelText string
	if len(removedLabels) == 1 {
		labelText = fmt.Sprintf("`%s`ラベル", removedLabels[0])
	} else {
		labelText = fmt.Sprintf("`%s`ラベルのいずれか", strings.Join(removedLabels, "`, `"))
	}

	message := fmt.Sprintf("🏷️ %sが削除されたため、レビュータスクを完了しました。", labelText)

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// PostPRClosedNotification は、PRがcloseされたことをスレッドに通知する
func PostPRClosedNotification(task models.ReviewTask, merged bool) error {
	if IsTestMode {
		log.Printf("test mode: would post PR closed notification for task: %s (merged: %v)", task.ID, merged)
		return nil
	}

	var message string
	if merged {
		message = "🎉 PRがマージされました！お疲れさまでした！"
	} else {
		message = "🔒 PRがクローズされました。"
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}
