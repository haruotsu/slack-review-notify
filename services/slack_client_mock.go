package services

import (
	"log"
	"slack-review-notify/models"

	"gorm.io/gorm"
)

// MockSlackClient はテスト用の Slack クライアントのモック実装
type MockSlackClient struct {
	// テスト用のフィールド（呼び出しを記録するため）
	SendMessageCalls                       []SendMessageCall
	SendMessageOffHoursCalls               []SendMessageOffHoursCall
	PostToThreadCalls                      []PostToThreadCall
	PostToThreadWithButtonsCalls           []PostToThreadWithButtonsCall
	PostBusinessHoursNotificationCalls     []PostBusinessHoursNotificationCall
	SendReviewerReminderCalls              []SendReviewerReminderCall
	SendReminderPausedCalls                []SendReminderPausedCall
	SendOutOfHoursReminderCalls            []SendOutOfHoursReminderCall
	PostReviewerAssignedCalls              []PostReviewerAssignedCall
	SendReviewerChangedCalls               []SendReviewerChangedCall
	UpdateSlackMessageForCompletedCalls    []UpdateSlackMessageCall
	SendReviewCompletedAutoNotificationCalls []SendReviewCompletedAutoNotificationCall
	PostLabelRemovedNotificationCalls      []PostLabelRemovedNotificationCall
	GetBotChannelsCalls                    int
	IsChannelArchivedCalls                 []IsChannelArchivedCall

	// モックの戻り値を設定するフィールド
	SendMessageResponse              SendMessageResponse
	SendMessageOffHoursResponse      SendMessageResponse
	GetBotChannelsResponse           GetBotChannelsResponse
	IsChannelArchivedResponse        IsChannelArchivedResponse
	DefaultError                     error
}

// 呼び出し記録用の構造体
type SendMessageCall struct {
	PRURL           string
	Title           string
	Channel         string
	MentionID       string
	CreatorSlackID  string
}

type SendMessageOffHoursCall struct {
	PRURL          string
	Title          string
	Channel        string
	CreatorSlackID string
}

type PostToThreadCall struct {
	Channel string
	TS      string
	Message string
}

type PostToThreadWithButtonsCall struct {
	Channel string
	TS      string
	Message string
	TaskID  string
}

type PostBusinessHoursNotificationCall struct {
	Task      models.ReviewTask
	MentionID string
}

type SendReviewerReminderCall struct {
	DB   *gorm.DB
	Task models.ReviewTask
}

type SendReminderPausedCall struct {
	Task     models.ReviewTask
	Duration string
}

type SendOutOfHoursReminderCall struct {
	DB   *gorm.DB
	Task models.ReviewTask
}

type PostReviewerAssignedCall struct {
	Task models.ReviewTask
}

type SendReviewerChangedCall struct {
	Task          models.ReviewTask
	OldReviewerID string
}

type UpdateSlackMessageCall struct {
	Task models.ReviewTask
}

type SendReviewCompletedAutoNotificationCall struct {
	Task        models.ReviewTask
	ReviewerLogin string
	ReviewState string
}

type PostLabelRemovedNotificationCall struct {
	Task          models.ReviewTask
	RemovedLabels []string
}

type IsChannelArchivedCall struct {
	ChannelID string
}

// レスポンス用の構造体
type SendMessageResponse struct {
	TS        string
	ChannelID string
	Error     error
}

type GetBotChannelsResponse struct {
	Channels []string
	Error    error
}

type IsChannelArchivedResponse struct {
	IsArchived bool
	Error      error
}

// NewMockSlackClient はモック Slack クライアントを作成する
func NewMockSlackClient() *MockSlackClient {
	return &MockSlackClient{
		SendMessageCalls:                       []SendMessageCall{},
		SendMessageOffHoursCalls:               []SendMessageOffHoursCall{},
		PostToThreadCalls:                      []PostToThreadCall{},
		PostToThreadWithButtonsCalls:           []PostToThreadWithButtonsCall{},
		PostBusinessHoursNotificationCalls:     []PostBusinessHoursNotificationCall{},
		SendReviewerReminderCalls:              []SendReviewerReminderCall{},
		SendReminderPausedCalls:                []SendReminderPausedCall{},
		SendOutOfHoursReminderCalls:            []SendOutOfHoursReminderCall{},
		PostReviewerAssignedCalls:              []PostReviewerAssignedCall{},
		SendReviewerChangedCalls:               []SendReviewerChangedCall{},
		UpdateSlackMessageForCompletedCalls:    []UpdateSlackMessageCall{},
		SendReviewCompletedAutoNotificationCalls: []SendReviewCompletedAutoNotificationCall{},
		PostLabelRemovedNotificationCalls:      []PostLabelRemovedNotificationCall{},
		IsChannelArchivedCalls:                 []IsChannelArchivedCall{},
		GetBotChannelsCalls:                    0,
		// デフォルトレスポンス
		SendMessageResponse: SendMessageResponse{
			TS:        "1234567890.123456",
			ChannelID: "C1234567890",
			Error:     nil,
		},
		SendMessageOffHoursResponse: SendMessageResponse{
			TS:        "1234567890.123456",
			ChannelID: "C1234567890",
			Error:     nil,
		},
		GetBotChannelsResponse: GetBotChannelsResponse{
			Channels: []string{"C1234567890"},
			Error:    nil,
		},
		IsChannelArchivedResponse: IsChannelArchivedResponse{
			IsArchived: false,
			Error:      nil,
		},
		DefaultError: nil,
	}
}

// SendSlackMessage のモック実装
func (m *MockSlackClient) SendSlackMessage(prURL, title, channel, mentionID, creatorSlackID string) (string, string, error) {
	m.SendMessageCalls = append(m.SendMessageCalls, SendMessageCall{
		PRURL:          prURL,
		Title:          title,
		Channel:        channel,
		MentionID:      mentionID,
		CreatorSlackID: creatorSlackID,
	})
	log.Printf("MockSlackClient: SendSlackMessage called (channel=%s, title=%s)", channel, title)
	return m.SendMessageResponse.TS, m.SendMessageResponse.ChannelID, m.SendMessageResponse.Error
}

// SendSlackMessageOffHours のモック実装
func (m *MockSlackClient) SendSlackMessageOffHours(prURL, title, channel, creatorSlackID string) (string, string, error) {
	m.SendMessageOffHoursCalls = append(m.SendMessageOffHoursCalls, SendMessageOffHoursCall{
		PRURL:          prURL,
		Title:          title,
		Channel:        channel,
		CreatorSlackID: creatorSlackID,
	})
	log.Printf("MockSlackClient: SendSlackMessageOffHours called (channel=%s, title=%s)", channel, title)
	return m.SendMessageOffHoursResponse.TS, m.SendMessageOffHoursResponse.ChannelID, m.SendMessageOffHoursResponse.Error
}

// PostToThread のモック実装
func (m *MockSlackClient) PostToThread(channel, ts, message string) error {
	m.PostToThreadCalls = append(m.PostToThreadCalls, PostToThreadCall{
		Channel: channel,
		TS:      ts,
		Message: message,
	})
	log.Printf("MockSlackClient: PostToThread called (channel=%s, ts=%s)", channel, ts)
	return m.DefaultError
}

// PostToThreadWithButtons のモック実装
func (m *MockSlackClient) PostToThreadWithButtons(channel, ts, message string, taskID string) error {
	m.PostToThreadWithButtonsCalls = append(m.PostToThreadWithButtonsCalls, PostToThreadWithButtonsCall{
		Channel: channel,
		TS:      ts,
		Message: message,
		TaskID:  taskID,
	})
	log.Printf("MockSlackClient: PostToThreadWithButtons called (channel=%s, ts=%s, taskID=%s)", channel, ts, taskID)
	return m.DefaultError
}

// PostBusinessHoursNotificationToThread のモック実装
func (m *MockSlackClient) PostBusinessHoursNotificationToThread(task models.ReviewTask, mentionID string) error {
	m.PostBusinessHoursNotificationCalls = append(m.PostBusinessHoursNotificationCalls, PostBusinessHoursNotificationCall{
		Task:      task,
		MentionID: mentionID,
	})
	log.Printf("MockSlackClient: PostBusinessHoursNotificationToThread called (taskID=%s)", task.ID)
	return m.DefaultError
}

// SendReviewerReminderMessage のモック実装
func (m *MockSlackClient) SendReviewerReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	m.SendReviewerReminderCalls = append(m.SendReviewerReminderCalls, SendReviewerReminderCall{
		DB:   db,
		Task: task,
	})
	log.Printf("MockSlackClient: SendReviewerReminderMessage called (taskID=%s)", task.ID)
	return m.DefaultError
}

// SendReminderPausedMessage のモック実装
func (m *MockSlackClient) SendReminderPausedMessage(task models.ReviewTask, duration string) error {
	m.SendReminderPausedCalls = append(m.SendReminderPausedCalls, SendReminderPausedCall{
		Task:     task,
		Duration: duration,
	})
	log.Printf("MockSlackClient: SendReminderPausedMessage called (taskID=%s, duration=%s)", task.ID, duration)
	return m.DefaultError
}

// SendOutOfHoursReminderMessage のモック実装
func (m *MockSlackClient) SendOutOfHoursReminderMessage(db *gorm.DB, task models.ReviewTask) error {
	m.SendOutOfHoursReminderCalls = append(m.SendOutOfHoursReminderCalls, SendOutOfHoursReminderCall{
		DB:   db,
		Task: task,
	})
	log.Printf("MockSlackClient: SendOutOfHoursReminderMessage called (taskID=%s)", task.ID)
	return m.DefaultError
}

// PostReviewerAssignedMessageWithChangeButton のモック実装
func (m *MockSlackClient) PostReviewerAssignedMessageWithChangeButton(task models.ReviewTask) error {
	m.PostReviewerAssignedCalls = append(m.PostReviewerAssignedCalls, PostReviewerAssignedCall{
		Task: task,
	})
	log.Printf("MockSlackClient: PostReviewerAssignedMessageWithChangeButton called (taskID=%s)", task.ID)
	return m.DefaultError
}

// SendReviewerChangedMessage のモック実装
func (m *MockSlackClient) SendReviewerChangedMessage(task models.ReviewTask, oldReviewerID string) error {
	m.SendReviewerChangedCalls = append(m.SendReviewerChangedCalls, SendReviewerChangedCall{
		Task:          task,
		OldReviewerID: oldReviewerID,
	})
	log.Printf("MockSlackClient: SendReviewerChangedMessage called (taskID=%s)", task.ID)
	return m.DefaultError
}

// UpdateSlackMessageForCompletedTask のモック実装
func (m *MockSlackClient) UpdateSlackMessageForCompletedTask(task models.ReviewTask) error {
	m.UpdateSlackMessageForCompletedCalls = append(m.UpdateSlackMessageForCompletedCalls, UpdateSlackMessageCall{
		Task: task,
	})
	log.Printf("MockSlackClient: UpdateSlackMessageForCompletedTask called (taskID=%s)", task.ID)
	return m.DefaultError
}

// SendReviewCompletedAutoNotification のモック実装
func (m *MockSlackClient) SendReviewCompletedAutoNotification(task models.ReviewTask, reviewerLogin string, reviewState string) error {
	m.SendReviewCompletedAutoNotificationCalls = append(m.SendReviewCompletedAutoNotificationCalls, SendReviewCompletedAutoNotificationCall{
		Task:          task,
		ReviewerLogin: reviewerLogin,
		ReviewState:   reviewState,
	})
	log.Printf("MockSlackClient: SendReviewCompletedAutoNotification called (taskID=%s, reviewer=%s, state=%s)", task.ID, reviewerLogin, reviewState)
	return m.DefaultError
}

// PostLabelRemovedNotification のモック実装
func (m *MockSlackClient) PostLabelRemovedNotification(task models.ReviewTask, removedLabels []string) error {
	m.PostLabelRemovedNotificationCalls = append(m.PostLabelRemovedNotificationCalls, PostLabelRemovedNotificationCall{
		Task:          task,
		RemovedLabels: removedLabels,
	})
	log.Printf("MockSlackClient: PostLabelRemovedNotification called (taskID=%s)", task.ID)
	return m.DefaultError
}

// GetBotChannels のモック実装
func (m *MockSlackClient) GetBotChannels() ([]string, error) {
	m.GetBotChannelsCalls++
	log.Printf("MockSlackClient: GetBotChannels called")
	return m.GetBotChannelsResponse.Channels, m.GetBotChannelsResponse.Error
}

// IsChannelArchived のモック実装
func (m *MockSlackClient) IsChannelArchived(channelID string) (bool, error) {
	m.IsChannelArchivedCalls = append(m.IsChannelArchivedCalls, IsChannelArchivedCall{
		ChannelID: channelID,
	})
	log.Printf("MockSlackClient: IsChannelArchived called (channelID=%s)", channelID)
	return m.IsChannelArchivedResponse.IsArchived, m.IsChannelArchivedResponse.Error
}
