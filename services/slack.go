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
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

// SlackAPIBaseURL returns the base URL for the Slack API.
// If the environment variable SLACK_API_BASE_URL is set, it uses that value;
// otherwise, it returns https://slack.com/api.
// Set this when using SlackHog (localhost:4112) in development environments.
func SlackAPIBaseURL() string {
	if base := os.Getenv("SLACK_API_BASE_URL"); base != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://slack.com/api"
}

// Flag indicating whether test mode is enabled
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
	// Always return validation success in test mode
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

	// Write body to the verifier
	if _, err := sv.Write(body); err != nil {
		log.Printf("Failed to write body to verifier: %v", err)
		return false
	}

	// Verify the signature
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

// GetAwayUserIDs returns the user IDs of users currently on leave
// (excludes scheduled future leaves where AwayFrom > now)
func GetAwayUserIDs(db *gorm.DB) []string {
	var records []models.ReviewerAvailability
	now := time.Now()

	// Retrieve records that are currently active:
	// 1. AwayFrom is nil (immediate) or in the past/present AND
	// 2. AwayUntil is nil (indefinite) or in the future
	result := db.Where(
		"(away_from IS NULL OR away_from <= ?) AND (away_until IS NULL OR away_until > ?)",
		now, now,
	).Find(&records)
	if result.Error != nil {
		log.Printf("failed to query away users: %v", result.Error)
	}

	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.SlackUserID)
	}
	return ids
}

// SelectRandomReviewers randomly selects the specified number of reviewers (excluding excludeIDs)
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

	// Also add users on leave to the exclusion list
	awayIDs := GetAwayUserIDs(db)

	excludeSet := make(map[string]bool)
	for _, id := range excludeIDs {
		if id != "" {
			excludeSet[id] = true
		}
	}
	for _, id := range awayIDs {
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

	if count <= 0 {
		return []string{}
	}
	if count > len(candidates) {
		count = len(candidates)
	}

	return candidates[:count]
}

// GetPendingReviewers returns the list of reviewers who have not yet approved
func GetPendingReviewers(task models.ReviewTask) []string {
	if task.Reviewers == "" {
		if task.Reviewer != "" {
			if isInCSV(task.ApprovedBy, task.Reviewer) {
				return nil
			}
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

// isInCSV checks whether the specified value is contained in a comma-separated string
func isInCSV(csv, value string) bool {
	if csv == "" || value == "" {
		return false
	}
	for _, id := range strings.Split(csv, ",") {
		if strings.TrimSpace(id) == value {
			return true
		}
	}
	return false
}

// AddApproval adds a reviewer to task.ApprovedBy (preventing duplicates). Returns true if newly added
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

// RemoveApproval removes the specified user from ApprovedBy. Returns true if removed.
func RemoveApproval(task *models.ReviewTask, slackUserID string) bool {
	if slackUserID == "" || task.ApprovedBy == "" {
		return false
	}

	var remaining []string
	found := false
	for _, id := range strings.Split(task.ApprovedBy, ",") {
		trimmed := strings.TrimSpace(id)
		if trimmed == slackUserID {
			found = true
		} else if trimmed != "" {
			remaining = append(remaining, trimmed)
		}
	}
	if !found {
		return false
	}
	task.ApprovedBy = strings.Join(remaining, ",")
	return true
}

// CountApprovals returns the number of approvals in task.ApprovedBy
func CountApprovals(task models.ReviewTask) int {
	if task.ApprovedBy == "" {
		return 0
	}
	count := 0
	for _, id := range strings.Split(task.ApprovedBy, ",") {
		if strings.TrimSpace(id) != "" {
			count++
		}
	}
	return count
}

// IsReviewFullyApproved determines whether the required number of approvals has been met.
// If the number of actually assigned reviewers is less than requiredApprovals, it uses the assigned count instead.
func IsReviewFullyApproved(task models.ReviewTask, requiredApprovals int) bool {
	if requiredApprovals <= 0 {
		requiredApprovals = 1
	}

	// Get the number of actually assigned reviewers
	if task.Reviewers != "" {
		assignedCount := 0
		for _, id := range strings.Split(task.Reviewers, ",") {
			if strings.TrimSpace(id) != "" {
				assignedCount++
			}
		}
		if assignedCount > 0 && assignedCount < requiredApprovals {
			requiredApprovals = assignedCount
		}
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

// SendSlackMessageOffHours sends a message without mentions for off-hours
func SendSlackMessageOffHours(prURL, title, channel, creatorSlackID, lang string) (string, string, error) {
	t := i18n.L(lang)
	var message string
	if creatorSlackID != "" {
		message = t("notify.off_hours.with_creator", creatorSlackID, title, prURL)
	} else {
		message = t("notify.off_hours.without_creator", title, prURL)
	}
	doneButton := CreateButton(t("button.review_done"), "review_done", "done", "primary")
	blocks := CreateMessageWithActionBlocks(message, doneButton)

	body := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// PostBusinessHoursNotificationToThread sends a notification with mentions to a thread when business hours begin
func PostBusinessHoursNotificationToThread(task models.ReviewTask, mentionID string) error {
	t := i18n.L(task.Language)
	mentionText := buildMentionText(mentionID)

	// Append reviewer info if a reviewer is assigned
	var reviewerText string
	if task.Reviewer != "" {
		reviewerText = t("notify.reviewer_in_morning", task.Reviewer)
	}

	message := t("notify.business_hours_morning", mentionText, reviewerText)

	blocks := CreateMessageBlocks(message)

	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

func SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID, lang string) (string, string, error) {
	t := i18n.L(lang)
	mentionText := buildMentionText(mentionID)

	var message string
	if creatorSlackID != "" {
		message = t("notify.review_request.with_creator", mentionText, creatorSlackID, title, prURL)
	} else {
		message = t("notify.review_request.without_creator", mentionText, title, prURL)
	}
	doneButton := CreateButton(t("button.review_done"), "review_done", "done", "primary")
	blocks := CreateMessageWithActionBlocks(message, doneButton)

	body := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// PostToThread posts a message to a thread
func PostToThread(channel, ts, message string) error {
	body := map[string]interface{}{
		"channel":   channel,
		"thread_ts": ts,
		"text":      message,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// PostToThreadWithButtons posts a message with buttons to a thread
func PostToThreadWithButtons(channel, ts, message string, taskID, lang string) error {
	t := i18n.L(lang)
	pauseButton := CreateButton(t("button.pause_reminder"), "pause_reminder_thread", taskID, "danger")
	blocks := CreateMessageWithActionBlocks(message, pauseButton)

	body := map[string]interface{}{
		"channel":   channel,
		"thread_ts": ts,
		"blocks":    blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// SendReviewerReminderMessage sends a reminder message to reviewers
func SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	t := i18n.L(task.Language)
	// Check if the channel is archived
	isArchived, err := IsChannelArchived(task.SlackChannel)
	if err != nil {
		log.Printf("channel status check error (channel: %s): %v", task.SlackChannel, err)

		// For Slack API errors, check the error type
		if strings.Contains(err.Error(), "not_in_channel") ||
			strings.Contains(err.Error(), "channel_not_found") {
			log.Printf("bot is not in channel or channel not found: %s", task.SlackChannel)

			// Deactivate the task
			task.Status = "archived"
			task.UpdatedAt = time.Now()
			db.Save(&task)

			// Also deactivate the channel config
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

		// Deactivate the task
		task.Status = "archived"
		task.UpdatedAt = time.Now()
		db.Save(&task)

		// Also deactivate the channel config
		var config models.ChannelConfig
		if result := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config); result.Error == nil {
			config.IsActive = false
			config.UpdatedAt = time.Now()
			db.Save(&config)
			log.Printf("channel %s config is deactivated", task.SlackChannel)
		}

		return fmt.Errorf("channel is archived: %s", task.SlackChannel)
	}

	// Mention only reviewers who have not yet approved
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
	message := t("notify.reminder", mentionText)

	pauseSelect := CreateAllOptionsPauseReminderSelect(task.ID, "pause_reminder", t("button.stop_reminder"), task.Language)
	blocks := CreateMessageWithActionBlocks(message, pauseSelect)

	// Post message with buttons to the thread
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// SendReminderPausedMessage notifies that the reminder has been paused
func SendReminderPausedMessage(task models.ReviewTask, duration string) error {
	t := i18n.L(task.Language)
	var message string

	switch duration {
	case "1h":
		message = t("notify.reminder_paused.1h")
	case "2h":
		message = t("notify.reminder_paused.2h")
	case "4h":
		message = t("notify.reminder_paused.4h")
	case "today":
		message = t("notify.reminder_paused.today")
	case "stop":
		message = t("notify.reminder_paused.stop")
	default:
		message = t("notify.reminder_paused.default")
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// GetBotChannels retrieves the list of channels the bot has joined
func GetBotChannels() ([]string, error) {
	url := SlackAPIBaseURL() + "/conversations.list?types=public_channel,private_channel"

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

// IsChannelRelatedError determines whether a Slack API error is a channel-related error
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

// IsChannelArchived checks whether a channel is archived
func IsChannelArchived(channelID string) (bool, error) {
	url := fmt.Sprintf("%s/conversations.info?channel=%s", SlackAPIBaseURL(), channelID)

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
			// Treat non-existent channels as archived
			return true, nil
		}
		return false, fmt.Errorf("failed to get channel info: %s", result.Error)
	}

	return result.Channel.IsArchived, nil
}

// PostReviewerAssignedMessageWithChangeButton displays the auto-assigned reviewers and shows a change button
func PostReviewerAssignedMessageWithChangeButton(task models.ReviewTask) error {
	t := i18n.L(task.Language)
	var message string
	if task.Reviewers != "" {
		var mentions []string
		for _, id := range strings.Split(task.Reviewers, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				mentions = append(mentions, fmt.Sprintf("<@%s>", trimmed))
			}
		}
		message = t("notify.reviewer_auto_assigned", strings.Join(mentions, " "))
	} else {
		message = t("notify.reviewer_auto_assigned", fmt.Sprintf("<@%s>", task.Reviewer))
	}

	changeButton := CreateChangeReviewerButton(task.ID, task.Language)
	pauseSelect := CreateAllOptionsPauseReminderSelect(task.ID, "pause_reminder_initial", t("button.stop_reminder"), task.Language)
	blocks := CreateMessageWithActionsBlocks(message, changeButton, pauseSelect)

	// Post message with buttons to the thread
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// formatReviewerMentions converts multiple reviewer IDs into Slack mention format
func formatReviewerMentions(reviewerIDs string) string {
	if reviewerIDs == "" {
		return ""
	}

	// Split by spaces to extract IDs
	ids := strings.Fields(reviewerIDs)

	var mentions []string
	for _, id := range ids {
		// Remove the @ symbol
		cleanID := strings.TrimPrefix(id, "@")
		if cleanID != "" {
			mentions = append(mentions, fmt.Sprintf("<@%s>", cleanID))
		}
	}

	return strings.Join(mentions, " ")
}

// SendReviewerChangedMessage notifies that the reviewer has been changed
func SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error {
	t := i18n.L(task.Language)
	oldMentions := formatReviewerMentions(oldReviewerID)
	newMentions := formatReviewerMentions(task.Reviewer)

	message := t("notify.reviewer_changed", oldMentions, newMentions)
	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// GetNextBusinessDayMorningWithConfig gets the next business day's opening time from the specified time (supports business hours config)
func GetNextBusinessDayMorningWithConfig(now time.Time, config *models.ChannelConfig) time.Time {
	// Get timezone setting
	timezone := "Asia/Tokyo"
	if config != nil && config.Timezone != "" {
		timezone = config.Timezone
	}

	// Load timezone
	tz, err := time.LoadLocation(timezone)
	if err != nil {
		// Fallback: use the current timezone
		tz = now.Location()
	}

	// Convert current time to the specified timezone
	nowInTZ := now.In(tz)

	// Get business hours start time (default: 10:00)
	businessHourStart := "10:00"
	if config != nil && config.BusinessHoursStart != "" {
		businessHourStart = config.BusinessHoursStart
	}

	// Parse business hours start time
	startHour, startMin, err := parseBusinessHoursTime(businessHourStart)
	if err != nil {
		// Default to 10:00 if parsing fails
		startHour, startMin = 10, 0
	}

	// Create today's business hours start time
	todayMorning := time.Date(nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(), startHour, startMin, 0, 0, tz)

	// Check current day of week and time
	weekday := nowInTZ.Weekday()

	// Variable to store the result
	var nextBusinessDayMorning time.Time

	switch weekday {
	case time.Monday, time.Tuesday, time.Wednesday, time.Thursday:
		// Monday through Thursday
		if nowInTZ.Before(todayMorning) {
			// Before business hours start: use today's start time
			nextBusinessDayMorning = todayMorning
		} else {
			// After business hours start: use tomorrow's start time
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
		}
	case time.Friday:
		// Friday
		if nowInTZ.Before(todayMorning) {
			// Before business hours start: use today's start time
			nextBusinessDayMorning = todayMorning
		} else {
			// After business hours start: use Monday's start time (3 days later)
			nextBusinessDayMorning = todayMorning.AddDate(0, 0, 3)
		}
	case time.Saturday:
		// Saturday: use Monday's start time (2 days later)
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 2)
	case time.Sunday:
		// Sunday: use Monday's start time (1 day later)
		nextBusinessDayMorning = todayMorning.AddDate(0, 0, 1)
	}

	return nextBusinessDayMorning
}

// SendOutOfHoursReminderMessage sends a reminder message for off-hours
func SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	t := i18n.L(task.Language)
	message := t("notify.out_of_hours_reminder", task.Reviewer)

	pauseSelect := CreateStopOnlyPauseReminderSelect(task.ID, "pause_reminder", t("button.stop_reminder"), task.Language)
	blocks := CreateMessageWithActionBlocks(message, pauseSelect)

	// Post message with buttons to the thread
	body := map[string]interface{}{
		"channel":   task.SlackChannel,
		"thread_ts": task.SlackTS,
		"blocks":    blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.postMessage", bytes.NewBuffer(jsonData))
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

// UpdateSlackMessageForCompletedTask updates the Slack message to indicate that the task is completed
func UpdateSlackMessageForCompletedTask(task models.ReviewTask) error {
	t := i18n.L(task.Language)
	if IsTestMode {
		log.Printf("test mode: would update slack message for completed task: %s", task.ID)
		return nil
	}

	message := t("notify.task_completed", task.Title, task.PRURL)
	blocks := CreateMessageBlocks(message)

	// Call the message update API
	body := map[string]interface{}{
		"channel": task.SlackChannel,
		"ts":      task.SlackTS,
		"blocks":  blocks,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+"/chat.update", bytes.NewBuffer(jsonData))
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

// SendReviewCompletedAutoNotification sends an automatic notification when a review is completed
func SendReviewCompletedAutoNotification(task models.ReviewTask, reviewerLogin string, reviewState string) error {
	t := i18n.L(task.Language)
	var message string
	var emoji string

	switch reviewState {
	case "approved":
		emoji = "✅"
		message = t("notify.review_approved", emoji, reviewerLogin)
	case "changes_requested":
		emoji = "🔄"
		message = t("notify.review_changes_requested", emoji, reviewerLogin)
	case "commented":
		emoji = "💬"
		message = t("notify.review_commented", emoji, reviewerLogin)
	default:
		emoji = "👀"
		message = t("notify.review_default", emoji, reviewerLogin)
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// PostLabelRemovedNotification notifies the thread about task completion due to label removal
func PostLabelRemovedNotification(task models.ReviewTask, removedLabels []string) error {
	t := i18n.L(task.Language)
	if IsTestMode {
		log.Printf("test mode: would post label removed notification for task: %s", task.ID)
		return nil
	}

	var message string
	if len(removedLabels) == 1 {
		message = t("notify.label_removed_single", removedLabels[0])
	} else {
		message = t("notify.label_removed_multiple", strings.Join(removedLabels, "`, `"))
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}

// PostPRClosedNotification notifies the thread that the PR has been closed
func PostPRClosedNotification(task models.ReviewTask, merged bool) error {
	t := i18n.L(task.Language)
	if IsTestMode {
		log.Printf("test mode: would post PR closed notification for task: %s (merged: %v)", task.ID, merged)
		return nil
	}

	var message string
	if merged {
		message = t("notify.pr_merged")
	} else {
		message = t("notify.pr_closed")
	}

	return PostToThread(task.SlackChannel, task.SlackTS, message)
}
