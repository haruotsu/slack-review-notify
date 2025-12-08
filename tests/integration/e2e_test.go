package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullWorkflow tests the complete flow: PR creation → reviewer notification → review completion → merge
func TestFullWorkflow(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Setup mock Slack client
	mockSlack := services.NewMockSlackClient()
	services.SetSlackClient(mockSlack)
	defer services.SetSlackClient(services.NewRealSlackClient())

	testChannelID := "C_E2E_WORKFLOW"
	testRepoFullName := "owner/e2e-test-repo"
	testPRNumber := 10001
	testLabelName := "needs-review"

	// Create test channel configuration with reviewers
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-%s", testChannelID, testLabelName),
		SlackChannelID:           testChannelID,
		LabelName:                testLabelName,
		DefaultMentionID:         "U_DEFAULT",
		ReviewerList:             "U_REVIEWER1,U_REVIEWER2,U_REVIEWER3",
		RepositoryList:           testRepoFullName,
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "00:00",
		BusinessHoursEnd:         "23:59",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Step 1: PR labeled event (PR creation with label)
	t.Log("Step 1: Sending PR labeled event...")
	prTitle := fmt.Sprintf("[E2E TEST] Full Workflow - %s", time.Now().Format("15:04:05"))
	prHTMLURL := fmt.Sprintf("https://github.com/%s/pull/%d", testRepoFullName, testPRNumber)
	action := "labeled"

	payload := github.PullRequestEvent{
		Action: &action,
		Number: &testPRNumber,
		Label: &github.Label{
			Name: &testLabelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &testPRNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
			User: &github.User{
				Login: github.Ptr("test-author"),
			},
			Labels: []*github.Label{
				{Name: &testLabelName},
			},
		},
		Repo: &github.Repository{
			Name:  github.Ptr("e2e-test-repo"),
			Owner: &github.User{Login: github.Ptr("owner")},
			FullName: github.Ptr(testRepoFullName),
		},
	}

	// Send labeled event
	sendWebhookEvent(t, router, "pull_request", payload)

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task was created
	var task models.ReviewTask
	err := testConfig.DB.Where("repo = ? AND pr_number = ? AND slack_channel = ?",
		testRepoFullName, testPRNumber, testChannelID).First(&task).Error
	require.NoError(t, err, "Task should be created after PR labeled event")

	assert.Equal(t, prHTMLURL, task.PRURL)
	assert.Equal(t, prTitle, task.Title)
	assert.Equal(t, "in_review", task.Status, "Task should be in_review status")
	assert.NotEmpty(t, task.Reviewer, "Reviewer should be assigned")
	assert.Contains(t, []string{"U_REVIEWER1", "U_REVIEWER2", "U_REVIEWER3"}, task.Reviewer,
		"Reviewer should be one of the configured reviewers")

	t.Logf("Task created successfully: ID=%s, Reviewer=%s", task.ID, task.Reviewer)

	// Step 2: Review submitted event
	t.Log("Step 2: Sending review submitted event...")
	reviewerLogin := "test-reviewer"
	reviewState := "approved"
	reviewAction := "submitted"

	reviewPayload := github.PullRequestReviewEvent{
		Action: &reviewAction,
		PullRequest: &github.PullRequest{
			Number: &testPRNumber,
		},
		Repo: &github.Repository{
			Name:  github.Ptr("e2e-test-repo"),
			Owner: &github.User{Login: github.Ptr("owner")},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  github.Ptr("LGTM! Looks good to merge."),
		},
	}

	// Send review submitted event
	sendWebhookEvent(t, router, "pull_request_review", reviewPayload)

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task was completed
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Task should still exist")

	assert.Equal(t, "completed", updatedTask.Status, "Task should be marked as completed after review")
	t.Logf("Task completed successfully after review")

	// Step 3: Unlabeled event (simulating label removal after merge)
	t.Log("Step 3: Sending unlabeled event (label removed after merge)...")
	unlabelAction := "unlabeled"

	unlabelPayload := github.PullRequestEvent{
		Action: &unlabelAction,
		Number: &testPRNumber,
		Label: &github.Label{
			Name: &testLabelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &testPRNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
			User: &github.User{
				Login: github.Ptr("test-author"),
			},
			Labels: []*github.Label{}, // No labels after unlabel
		},
		Repo: &github.Repository{
			Name:  github.Ptr("e2e-test-repo"),
			Owner: &github.User{Login: github.Ptr("owner")},
			FullName: github.Ptr(testRepoFullName),
		},
	}

	// Send unlabeled event
	sendWebhookEvent(t, router, "pull_request", unlabelPayload)

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task remains completed
	var finalTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&finalTask).Error
	require.NoError(t, err, "Task should still exist")

	assert.Equal(t, "completed", finalTask.Status, "Task should remain completed")
	t.Logf("Full workflow completed successfully")

	// Cleanup: delete task
	testConfig.DB.Delete(&task)
}

// TestMultipleReviewers tests the flow with multiple reviewers being assigned
func TestMultipleReviewers(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Setup mock Slack client
	mockSlack := services.NewMockSlackClient()
	services.SetSlackClient(mockSlack)
	defer services.SetSlackClient(services.NewRealSlackClient())

	testChannelID := "C_E2E_MULTI_REVIEWERS"
	testRepoFullName := "owner/multi-reviewer-repo"
	testLabelName := "needs-review"

	// Create test channel configuration with multiple reviewers
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-%s", testChannelID, testLabelName),
		SlackChannelID:           testChannelID,
		LabelName:                testLabelName,
		DefaultMentionID:         "U_DEFAULT",
		ReviewerList:             "U_ALICE,U_BOB,U_CHARLIE,U_DAVID",
		RepositoryList:           testRepoFullName,
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "00:00",
		BusinessHoursEnd:         "23:59",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Test multiple PRs to verify different reviewers are assigned
	reviewerAssignments := make(map[string]int)

	for i := 0; i < 10; i++ {
		prNumber := 20001 + i
		prTitle := fmt.Sprintf("[E2E TEST] Multiple Reviewers Test %d - %s", i+1, time.Now().Format("15:04:05.000"))
		prHTMLURL := fmt.Sprintf("https://github.com/%s/pull/%d", testRepoFullName, prNumber)
		action := "labeled"

		payload := github.PullRequestEvent{
			Action: &action,
			Number: &prNumber,
			Label: &github.Label{
				Name: &testLabelName,
			},
			PullRequest: &github.PullRequest{
				Number:  &prNumber,
				Title:   &prTitle,
				HTMLURL: &prHTMLURL,
				User: &github.User{
					Login: github.Ptr("test-author"),
				},
				Labels: []*github.Label{
					{Name: &testLabelName},
				},
			},
			Repo: &github.Repository{
				Name:  github.Ptr("multi-reviewer-repo"),
				Owner: &github.User{Login: github.Ptr("owner")},
				FullName: github.Ptr(testRepoFullName),
			},
		}

		// Send labeled event
		sendWebhookEvent(t, router, "pull_request", payload)

		// Wait for background processing
		time.Sleep(1 * time.Second)

		// Verify task was created and reviewer assigned
		var task models.ReviewTask
		err := testConfig.DB.Where("repo = ? AND pr_number = ? AND slack_channel = ?",
			testRepoFullName, prNumber, testChannelID).First(&task).Error
		require.NoError(t, err, "Task should be created for PR %d", prNumber)

		assert.NotEmpty(t, task.Reviewer, "Reviewer should be assigned for PR %d", prNumber)
		assert.Contains(t, []string{"U_ALICE", "U_BOB", "U_CHARLIE", "U_DAVID"}, task.Reviewer,
			"Reviewer should be one of the configured reviewers")

		// Track reviewer assignments
		reviewerAssignments[task.Reviewer]++

		t.Logf("PR %d: Reviewer assigned: %s", prNumber, task.Reviewer)

		// Cleanup: delete task
		testConfig.DB.Delete(&task)
	}

	// Verify that multiple different reviewers were assigned (not always the same one)
	t.Logf("Reviewer assignment distribution: %v", reviewerAssignments)
	assert.GreaterOrEqual(t, len(reviewerAssignments), 2,
		"At least 2 different reviewers should have been assigned across 10 PRs")
}

// TestReviewerChange tests the flow of changing a reviewer
func TestReviewerChange(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Setup mock Slack client
	mockSlack := services.NewMockSlackClient()
	services.SetSlackClient(mockSlack)
	defer services.SetSlackClient(services.NewRealSlackClient())

	testChannelID := "C_E2E_REVIEWER_CHANGE"
	testRepoFullName := "owner/reviewer-change-repo"
	testPRNumber := 30001
	testLabelName := "needs-review"

	// Create test channel configuration with reviewers
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-%s", testChannelID, testLabelName),
		SlackChannelID:           testChannelID,
		LabelName:                testLabelName,
		DefaultMentionID:         "U_DEFAULT",
		ReviewerList:             "U_ALICE,U_BOB,U_CHARLIE",
		RepositoryList:           testRepoFullName,
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "00:00",
		BusinessHoursEnd:         "23:59",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Step 1: Create PR with label
	t.Log("Step 1: Creating PR with label...")
	prTitle := fmt.Sprintf("[E2E TEST] Reviewer Change - %s", time.Now().Format("15:04:05"))
	prHTMLURL := fmt.Sprintf("https://github.com/%s/pull/%d", testRepoFullName, testPRNumber)
	action := "labeled"

	payload := github.PullRequestEvent{
		Action: &action,
		Number: &testPRNumber,
		Label: &github.Label{
			Name: &testLabelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &testPRNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
			User: &github.User{
				Login: github.Ptr("test-author"),
			},
			Labels: []*github.Label{
				{Name: &testLabelName},
			},
		},
		Repo: &github.Repository{
			Name:  github.Ptr("reviewer-change-repo"),
			Owner: &github.User{Login: github.Ptr("owner")},
			FullName: github.Ptr(testRepoFullName),
		},
	}

	// Send labeled event
	sendWebhookEvent(t, router, "pull_request", payload)

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task was created with initial reviewer
	var task models.ReviewTask
	err := testConfig.DB.Where("repo = ? AND pr_number = ? AND slack_channel = ?",
		testRepoFullName, testPRNumber, testChannelID).First(&task).Error
	require.NoError(t, err, "Task should be created")

	initialReviewer := task.Reviewer
	assert.NotEmpty(t, initialReviewer, "Initial reviewer should be assigned")
	t.Logf("Initial reviewer assigned: %s", initialReviewer)

	// Step 2: Change reviewer directly in database (simulating Slack button interaction)
	t.Log("Step 2: Changing reviewer...")

	// Select a different reviewer
	newReviewer := "U_ALICE"
	if initialReviewer == "U_ALICE" {
		newReviewer = "U_BOB"
	}

	task.Reviewer = newReviewer
	task.UpdatedAt = time.Now()
	err = testConfig.DB.Save(&task).Error
	require.NoError(t, err, "Should be able to update reviewer")

	t.Logf("Reviewer changed from %s to %s", initialReviewer, newReviewer)

	// Verify reviewer was changed
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Task should exist")

	assert.Equal(t, newReviewer, updatedTask.Reviewer, "Reviewer should be updated")
	assert.NotEqual(t, initialReviewer, updatedTask.Reviewer, "Reviewer should be different from initial")

	// Step 3: Verify task can still be completed with new reviewer
	t.Log("Step 3: Completing review with new reviewer...")
	reviewerLogin := "new-reviewer"
	reviewState := "approved"
	reviewAction := "submitted"

	reviewPayload := github.PullRequestReviewEvent{
		Action: &reviewAction,
		PullRequest: &github.PullRequest{
			Number: &testPRNumber,
		},
		Repo: &github.Repository{
			Name:  github.Ptr("reviewer-change-repo"),
			Owner: &github.User{Login: github.Ptr("owner")},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  github.Ptr("LGTM!"),
		},
	}

	// Send review submitted event
	sendWebhookEvent(t, router, "pull_request_review", reviewPayload)

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task was completed
	var finalTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&finalTask).Error
	require.NoError(t, err, "Task should exist")

	assert.Equal(t, "completed", finalTask.Status, "Task should be completed")
	assert.Equal(t, newReviewer, finalTask.Reviewer, "Reviewer should remain as the new reviewer")
	t.Logf("Reviewer change flow completed successfully")

	// Cleanup: delete task
	testConfig.DB.Delete(&task)
}

// Helper function to send webhook events
func sendWebhookEvent(t *testing.T, router *gin.Engine, eventType string, payload interface{}) {
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
	require.NoError(t, err, "Should create request")

	// Sign the request if GITHUB_WEBHOOK_SECRET is set
	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		signature := signWebhookPayload(payloadJSON, []byte(secret))
		req.Header.Set("X-Hub-Signature-256", signature)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", eventType)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")
}

// Helper function to sign webhook payload
func signWebhookPayload(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + signature
}
