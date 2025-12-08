package services

import (
	"slack-review-notify/models"

	"gorm.io/gorm"
)

// SlackClient は Slack API クライアントのインターフェース
type SlackClient interface {
	// メッセージ送信系
	SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID string) (ts string, channelID string, err error)
	SendSlackMessageOffHours(prURL, title, channel, creatorSlackID string) (ts string, channelID string, err error)
	PostToThread(channel, ts, message string) error
	PostToThreadWithButtons(channel, ts, message string, taskID string) error
	PostBusinessHoursNotificationToThread(task models.ReviewTask, mentionID string) error

	// リマインダー系
	SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error
	SendReminderPausedMessage(task models.ReviewTask, duration string) error
	SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error

	// レビュワー関連
	PostReviewerAssignedMessageWithChangeButton(task models.ReviewTask) error
	SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error

	// タスク完了関連
	UpdateSlackMessageForCompletedTask(task models.ReviewTask) error
	SendReviewCompletedAutoNotification(task models.ReviewTask, reviewerLogin string, reviewState string) error
	PostLabelRemovedNotification(task models.ReviewTask, removedLabels []string) error

	// チャンネル情報取得
	GetBotChannels() ([]string, error)
	IsChannelArchived(channelID string) (bool, error)
}

// RealSlackClient は実際の Slack API を呼び出すクライアント
type RealSlackClient struct{}

// NewRealSlackClient は実際の Slack クライアントを作成する
func NewRealSlackClient() SlackClient {
	return &RealSlackClient{}
}

// 以下、RealSlackClient の各メソッドは既存の関数を呼び出す

func (c *RealSlackClient) SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID string) (string, string, error) {
	return SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID)
}

func (c *RealSlackClient) SendSlackMessageOffHours(prURL, title, channel, creatorSlackID string) (string, string, error) {
	return SendSlackMessageOffHours(prURL, title, channel, creatorSlackID)
}

func (c *RealSlackClient) PostToThread(channel, ts, message string) error {
	return PostToThread(channel, ts, message)
}

func (c *RealSlackClient) PostToThreadWithButtons(channel, ts, message string, taskID string) error {
	return PostToThreadWithButtons(channel, ts, message, taskID)
}

func (c *RealSlackClient) PostBusinessHoursNotificationToThread(task models.ReviewTask, mentionID string) error {
	return PostBusinessHoursNotificationToThread(task, mentionID)
}

func (c *RealSlackClient) SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	return SendReviewerReminderMessage(db, task)
}

func (c *RealSlackClient) SendReminderPausedMessage(task models.ReviewTask, duration string) error {
	return SendReminderPausedMessage(task, duration)
}

func (c *RealSlackClient) SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	return SendOutOfHoursReminderMessage(db, task)
}

func (c *RealSlackClient) PostReviewerAssignedMessageWithChangeButton(task models.ReviewTask) error {
	return PostReviewerAssignedMessageWithChangeButton(task)
}

func (c *RealSlackClient) SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error {
	return SendReviewerChangedMessage(task, oldReviewerID)
}

func (c *RealSlackClient) UpdateSlackMessageForCompletedTask(task models.ReviewTask) error {
	return UpdateSlackMessageForCompletedTask(task)
}

func (c *RealSlackClient) SendReviewCompletedAutoNotification(task models.ReviewTask, reviewerLogin string, reviewState string) error {
	return SendReviewCompletedAutoNotification(task, reviewerLogin, reviewState)
}

func (c *RealSlackClient) PostLabelRemovedNotification(task models.ReviewTask, removedLabels []string) error {
	return PostLabelRemovedNotification(task, removedLabels)
}

func (c *RealSlackClient) GetBotChannels() ([]string, error) {
	return GetBotChannels()
}

func (c *RealSlackClient) IsChannelArchived(channelID string) (bool, error) {
	return IsChannelArchived(channelID)
}
