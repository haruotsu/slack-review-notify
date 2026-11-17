package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedMultiReviewerTask(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&models.ChannelConfig{
		ID:             "config-multi",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		ReviewerList:   "UAAA,UBBB,UCCC,UDDD",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ReviewTask{
		ID:              "task-multi",
		PRURL:           "https://github.com/test/repo/pull/1",
		Repo:            "test/repo",
		PRNumber:        1,
		Title:           "Test PR",
		SlackTS:         "1234.5678",
		SlackChannel:    "C12345",
		Status:          "in_review",
		Reviewer:        "UAAA",
		Reviewers:       "UAAA,UBBB",
		PRAuthorSlackID: "UAUTHOR",
		LabelName:       "needs-review",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}).Error)
}

func postSlackAction(t *testing.T, db *gorm.DB, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/actions", HandleSlackAction(db))

	b, err := json.Marshal(payload)
	require.NoError(t, err)
	form := url.Values{}
	form.Set("payload", string(b))

	req := httptest.NewRequest("POST", "/slack/actions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func changeReviewerSubmission(taskID, target, newUser string) map[string]any {
	return map[string]any{
		"type": "view_submission",
		"user": map[string]any{"id": "U99999"},
		"view": map[string]any{
			"id":               "V123",
			"callback_id":      services.ChangeReviewerModalCallbackID,
			"private_metadata": services.EncodeChangeReviewerModalMetadata(services.ChangeReviewerModalMetadata{TaskID: taskID}),
			"state": map[string]any{"values": map[string]any{
				services.ChangeReviewerTargetBlockID: map[string]any{
					services.ChangeReviewerTargetBlockID: map[string]any{
						"type":            "radio_buttons",
						"selected_option": map[string]any{"value": target},
					},
				},
				services.ChangeReviewerNewBlockID: map[string]any{
					services.ChangeReviewerNewBlockID: map[string]any{
						"type":          "users_select",
						"selected_user": newUser,
					},
				},
			}},
		},
	}
}

// singleReviewerSubmission mirrors what Slack posts for the single-reviewer
// modal: the target lives only in private_metadata, so there is no target block.
func singleReviewerSubmission(taskID, fixedTarget, newUser string) map[string]any {
	p := changeReviewerSubmission(taskID, "", newUser)
	view := p["view"].(map[string]any)
	view["private_metadata"] = services.EncodeChangeReviewerModalMetadata(services.ChangeReviewerModalMetadata{TaskID: taskID, FixedTarget: fixedTarget})
	delete(view["state"].(map[string]any)["values"].(map[string]any), services.ChangeReviewerTargetBlockID)
	return p
}

func submitChangeReviewerModal(t *testing.T, db *gorm.DB, taskID, fixedTarget, newUser string) {
	t.Helper()
	w := postSlackAction(t, db, singleReviewerSubmission(taskID, fixedTarget, newUser))
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "response_action", "モーダル送信がエラーにならないこと: %s", w.Body.String())
}

func seedSingleReviewerTask(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedMultiReviewerTask(t, db)
	require.NoError(t, db.Model(&models.ReviewTask{}).Where("id = ?", "task-multi").
		Updates(map[string]any{"reviewers": "UAAA"}).Error)
}

func TestChangeReviewerButton_SingleReviewer_OpensModalWithoutReassigning(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedSingleReviewerTask(t, db)

	w := postSlackAction(t, db, map[string]any{
		"type":       "block_actions",
		"trigger_id": "trigger-1",
		"user":       map[string]any{"id": "U99999"},
		"actions":    []map[string]any{{"action_id": "change_reviewer", "value": "task-multi"}},
		"container":  map[string]any{"channel_id": "C12345"},
		"message":    map[string]any{"ts": "1234.5678"},
	})

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA", got.Reviewers)
	assert.Equal(t, "UAAA", got.Reviewer)
}

func TestChangeReviewerModal_SingleReviewer_ManualSuccessor(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedSingleReviewerTask(t, db)

	submitChangeReviewerModal(t, db, "task-multi", "UAAA", "UOUTSIDER")

	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UOUTSIDER", got.Reviewers)
	assert.Equal(t, "UOUTSIDER", got.Reviewer)
}

func TestChangeReviewerModal_SingleReviewer_BlankSuccessorDraws(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedSingleReviewerTask(t, db)

	submitChangeReviewerModal(t, db, "task-multi", "UAAA", "")

	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Contains(t, []string{"UBBB", "UCCC", "UDDD"}, got.Reviewer)
	assert.Equal(t, got.Reviewer, got.Reviewers)
}

func TestChangeReviewerModal_SingleReviewer_StaleTargetShowsErrorOnSuccessorBlock(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedSingleReviewerTask(t, db)

	w := postSlackAction(t, db, singleReviewerSubmission("task-multi", "UGONE", "UCCC"))

	assert.Contains(t, w.Body.String(), `"response_action":"errors"`)
	assert.Contains(t, w.Body.String(), services.ChangeReviewerNewBlockID,
		"交代対象の入力欄が無いモーダルでは、後任欄にエラーを出す")
	assert.NotContains(t, w.Body.String(), services.ChangeReviewerTargetBlockID)
}

func TestChangeReviewerButton_MultipleReviewers_OpensModalWithoutReassigning(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)

	w := postSlackAction(t, db, map[string]any{
		"type":       "block_actions",
		"trigger_id": "trigger-1",
		"user":       map[string]any{"id": "U99999"},
		"actions":    []map[string]any{{"action_id": "change_reviewer", "value": "task-multi"}},
		"container":  map[string]any{"channel_id": "C12345"},
		"message":    map[string]any{"ts": "1234.5678"},
	})

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UBBB", got.Reviewers, "モーダルで指定されるまでレビュワーは変わらない")
	assert.Equal(t, "UAAA", got.Reviewer)
}

func TestChangeReviewerModal_Submission_ReplacesOnlySelectedReviewer(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", "UCCC"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "response_action")
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UCCC", got.Reviewers)
	assert.Equal(t, "UAAA", got.Reviewer)
}

func TestChangeReviewerModal_Submission_AllowsUserOutsideReviewerList(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UAAA", "UOUTSIDER"))

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UOUTSIDER,UBBB", got.Reviewers)
	assert.Equal(t, "UOUTSIDER", got.Reviewer)
}

func TestChangeReviewerModal_Submission_BlankSuccessorDrawsFromReviewerList(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", ""))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "response_action")
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	reviewers := strings.Split(got.Reviewers, ",")
	require.Len(t, reviewers, 2)
	assert.Equal(t, "UAAA", reviewers[0], "交代対象でないレビュワーはそのまま")
	assert.Contains(t, []string{"UCCC", "UDDD"}, reviewers[1], "未アサインのリスト内メンバーから抽選される")
}

func TestChangeReviewerModal_Submission_BlankSuccessorWithNoCandidate(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)
	require.NoError(t, db.Model(&models.ChannelConfig{}).Where("id = ?", "config-multi").
		Update("reviewer_list", "UAAA,UBBB,UAUTHOR").Error)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", ""))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"response_action":"errors"`)
	assert.Contains(t, w.Body.String(), services.ChangeReviewerNewBlockID)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UBBB", got.Reviewers)
}

func TestChangeReviewerModal_Submission_BlankSuccessorWithoutReviewerListIsRejected(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)
	require.NoError(t, db.Model(&models.ChannelConfig{}).Where("id = ?", "config-multi").
		Updates(map[string]any{"reviewer_list": "", "default_mention_id": "UTEAM"}).Error)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", ""))

	assert.Contains(t, w.Body.String(), `"response_action":"errors"`,
		"レビュワーリストが無いとき、個人の枠をチャンネル全体へのメンションで埋めない")
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UBBB", got.Reviewers)
}

func TestChangeReviewerModal_Submission_KeepsApprovals(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)
	require.NoError(t, db.Model(&models.ReviewTask{}).Where("id = ?", "task-multi").
		Update("approved_by", "UAAA").Error)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", "UCCC"))

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UCCC", got.Reviewers)
	assert.Equal(t, "UAAA", got.ApprovedBy)
}

func TestChangeReviewerButton_OverRadioButtonLimit_RedrawsAtRandom(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	eleven := "U01,U02,U03,U04,U05,U06,U07,U08,U09,U10,U11"
	require.NoError(t, db.Create(&models.ChannelConfig{
		ID: "config-big", SlackChannelID: "C12345", LabelName: "needs-review",
		ReviewerList: eleven + ",U12", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ReviewTask{
		ID: "task-big", PRURL: "https://github.com/test/repo/pull/2", Repo: "test/repo", PRNumber: 2,
		Title: "Big PR", SlackTS: "1234.5678", SlackChannel: "C12345", Status: "in_review",
		Reviewer: "U01", Reviewers: eleven, LabelName: "needs-review", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	w := postSlackAction(t, db, map[string]any{
		"type":       "block_actions",
		"trigger_id": "trigger-1",
		"user":       map[string]any{"id": "U99999"},
		"actions":    []map[string]any{{"action_id": "change_reviewer", "value": "task-big"}},
		"container":  map[string]any{"channel_id": "C12345"},
		"message":    map[string]any{"ts": "1234.5678"},
	})

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-big").First(&got).Error)
	assert.Equal(t, "U12", got.Reviewer, "モーダルを描画できない人数ではワンクリック再抽選のまま")
	assert.True(t, strings.HasPrefix(got.Reviewers, "U12,"))
}

func TestChangeReviewerModal_Submission_ValidationErrorKeepsTask(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	seedMultiReviewerTask(t, db)

	w := postSlackAction(t, db, changeReviewerSubmission("task-multi", "UBBB", "UAUTHOR"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"response_action":"errors"`)
	assert.Contains(t, w.Body.String(), services.ChangeReviewerNewBlockID)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&got).Error)
	assert.Equal(t, "UAAA,UBBB", got.Reviewers)
}

func TestChangeReviewerModal_Submission_UnknownTask(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)

	w := postSlackAction(t, db, changeReviewerSubmission("no-such-task", "UBBB", "UCCC"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"response_action":"errors"`)
}
