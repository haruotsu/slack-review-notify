package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// runAsync runs Slack API calls off the request goroutine. Tests swap it for a
// synchronous runner: a goroutine outliving the test would read IsTestMode
// after the test restored it and hit the real Slack API.
var runAsync = func(f func()) { go f() }

func handleOpenChangeReviewerModal(c *gin.Context, payload SlackActionPayload, task models.ReviewTask) {
	view := services.BuildChangeReviewerModalView(task)
	triggerID := payload.TriggerID

	runAsync(func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("change-reviewer views.open failed: task=%s err=%v", task.ID, err)
		}
	})

	c.Status(http.StatusOK)
}

func handleChangeReviewerModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	respondErr := func(blockID, msg string) {
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{blockID: msg},
		})
	}

	meta, err := services.DecodeChangeReviewerModalMetadata(payload.View.PrivateMetadata)
	if err != nil || meta.TaskID == "" {
		log.Printf("change-reviewer view_submission has invalid private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		respondErr(services.ChangeReviewerNewBlockID, i18n.T("modal.change_reviewer.error.task_not_found"))
		return
	}

	var task models.ReviewTask
	if err := db.Where("id = ?", meta.TaskID).First(&task).Error; err != nil {
		log.Printf("change-reviewer: task id %s not found: %v", meta.TaskID, err)
		respondErr(services.ChangeReviewerNewBlockID, i18n.T("modal.change_reviewer.error.task_not_found"))
		return
	}

	form, err := services.ParseChangeReviewerModalSubmission(payload.View.State.Values, task, meta)
	if err != nil {
		var ve *services.ModalValidationError
		if errors.As(err, &ve) {
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          ve.Errors,
			})
			return
		}
		log.Printf("change-reviewer parse error: %v", err)
		respondErr(services.ChangeReviewerNewBlockID, i18n.TWithLang(task.Language, "modal.change_reviewer.error.save_failed"))
		return
	}

	newReviewerID := form.NewReviewerID
	if newReviewerID == "" {
		drawn, ok := services.DrawReplacementReviewer(db, task, true)
		if !ok {
			respondErr(services.ChangeReviewerNewBlockID, i18n.TWithLang(task.Language, "modal.change_reviewer.error.no_candidate"))
			return
		}
		newReviewerID = drawn
	}

	prevReviewers, prevReviewer := task.Reviewers, task.Reviewer
	services.ReplaceTaskReviewer(&task, form.OldReviewerID, newReviewerID)
	task.UpdatedAt = time.Now()

	// Not db.Save: writing the whole row back would clobber an approval that
	// the GitHub webhook recorded while the modal was open. The reviewer
	// columns in the WHERE make a concurrent swap by someone else fail
	// instead of being silently overwritten.
	// COALESCE because rows created before the reviewers column was added hold
	// NULL there, which GORM reads back as "" but `reviewers = ''` never matches.
	res := db.Model(&models.ReviewTask{}).
		Where("id = ? AND COALESCE(reviewers, '') = ? AND COALESCE(reviewer, '') = ?", task.ID, prevReviewers, prevReviewer).
		Updates(map[string]any{
			"reviewers":  task.Reviewers,
			"reviewer":   task.Reviewer,
			"updated_at": task.UpdatedAt,
		})
	if res.Error != nil {
		log.Printf("change-reviewer: save failed: task=%s err=%v", task.ID, res.Error)
		respondErr(services.ChangeReviewerNewBlockID, i18n.TWithLang(task.Language, "modal.change_reviewer.error.save_failed"))
		return
	}
	if res.RowsAffected == 0 {
		log.Printf("change-reviewer: reviewers changed concurrently: task=%s expected reviewers=%q reviewer=%q", task.ID, prevReviewers, prevReviewer)
		respondErr(meta.ErrorBlockForTarget(), i18n.TWithLang(task.Language, "modal.change_reviewer.error.target_not_assigned"))
		return
	}

	// view_submission must be answered within 3 seconds; a slow chat.postMessage
	// would make Slack show an error even though the swap is already saved.
	oldReviewerID := form.OldReviewerID
	runAsync(func() {
		if err := services.SendReviewerChangedMessage(task, oldReviewerID, newReviewerID); err != nil {
			log.Printf("reviewer change notification error: %v", err)
		}
	})

	c.Status(http.StatusOK)
}
