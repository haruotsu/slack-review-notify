package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func menuActionPayload(t *testing.T, op, channelID, userID string) SlackActionPayload {
	t.Helper()
	raw := fmt.Sprintf(`{
		"type": "block_actions",
		"trigger_id": "trigger-xyz",
		"user": {"id": %q},
		"actions": [{"action_id": %q, "value": %q}],
		"container": {"type": "view"},
		"view": {"id": "V1", "callback_id": "menu_modal", "private_metadata": %q}
	}`, userID, services.MenuActionPrefix+op, op,
		services.EncodeMenuModalMetadata(services.MenuModalMetadata{ChannelID: channelID, UserID: "Uopener"}))

	var payload SlackActionPayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}

func viewText(t *testing.T, view map[string]any) string {
	t.Helper()
	b, err := json.Marshal(view)
	require.NoError(t, err)
	return string(b)
}

func TestMenuCommand_OpensTheMenuWithoutPostingAMessage(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "menu")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")
	form.Add("trigger_id", "trigger-xyz")
	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String(), "the modal is the response; any body would also post a message")
}

func TestHelpCommand_IncludesOpenMenuButton(t *testing.T) {
	db := setupTestDB(t)

	resp := postHelpCommand(t, db, "C12345")

	var actionIDs []string
	for _, block := range resp.Blocks {
		for _, el := range block.Elements {
			actionIDs = append(actionIDs, el.ActionID)
		}
	}
	assert.Contains(t, actionIDs, services.OpenMenuActionID)
}

func TestMenuActionView_EditingOperationsOpenTheirExistingModals(t *testing.T) {
	db := setupTestDB(t)

	cases := map[string]string{
		services.MenuOpEditSettings: services.SettingsModalCallbackID,
		services.MenuOpNewLabel:     services.SettingsModalCallbackID,
		services.MenuOpAway:         services.AwayManagementModalCallbackID,
		services.MenuOpUserMapping:  services.UserMappingModalCallbackID,
	}
	for op, callbackID := range cases {
		t.Run(op, func(t *testing.T) {
			view, ok := menuActionView(db, menuActionPayload(t, op, "C12345", "U1"))
			require.True(t, ok)
			assert.Equal(t, callbackID, view["callback_id"])
			// The pushed modal posts its confirmation to the channel in its own
			// metadata, which must be the one the menu was opened from.
			assert.Contains(t, view["private_metadata"], `"c":"C12345"`)
		})
	}
}

func TestMenuActionView_EveryMenuButtonOpensAView(t *testing.T) {
	db := setupTestDB(t)

	for _, op := range append(append([]string{}, services.MenuShowOps...), services.MenuConfigOps...) {
		t.Run(op, func(t *testing.T) {
			view, ok := menuActionView(db, menuActionPayload(t, op, "C12345", "U1"))
			require.True(t, ok)
			assert.NotEmpty(t, view["blocks"])
		})
	}
}

func TestMenuActionView_EditSettingsStartsOnAnExistingLabel(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "bug", DefaultMentionID: "UBUGOWNER"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpEditSettings, "C12345", "U1"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "UBUGOWNER", "the only configured label must be preselected and prefilled")
	assert.NotContains(t, text, `"block_id":"new_label_name"`)
}

func TestMenuActionView_NewLabelStartsInCreateMode(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpNewLabel, "C12345", "U1"))

	require.True(t, ok)
	assert.Contains(t, viewText(t, view), `"block_id":"new_label_name"`)
}

func TestMenuActionView_LabelDetailsCoverEveryLabelOfThisChannelOnly(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review", RepositoryList: "owner/app"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c2", SlackChannelID: "C12345", LabelName: "bug", RepositoryList: "owner/api"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c3", SlackChannelID: "COTHER", LabelName: "secret", RepositoryList: "owner/secret"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowLabelDetails, "C12345", "U1"))

	require.True(t, ok)
	assert.Equal(t, services.MenuResultCallbackID, view["callback_id"])
	text := viewText(t, view)
	assert.Contains(t, text, "owner/app")
	assert.Contains(t, text, "owner/api")
	assert.NotContains(t, text, "owner/secret")
}

func TestMenuActionView_ReviewersAreListedPerLabel(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review", ReviewerList: "UREVIEWER1"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c2", SlackChannelID: "C12345", LabelName: "bug", ReviewerList: "UREVIEWER2"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowReviewers, "C12345", "U1"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "UREVIEWER1")
	assert.Contains(t, text, "UREVIEWER2")
}

func TestMenuActionView_PerLabelViewsExplainWhenNothingIsConfigured(t *testing.T) {
	db := setupTestDB(t)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowReviewers, "C12345", "U1"))

	require.True(t, ok)
	assert.Contains(t, viewText(t, view), "このチャンネルにはまだ設定がありません")
}

// The menu may be clicked by someone other than whoever the metadata recorded,
// and "my reviews" must be the clicker's.
func TestMenuActionView_MyReviewsBelongToWhoeverClicked(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ReviewTask{ID: "t1", Repo: "owner/mine", PRNumber: 1, Status: "in_review", Reviewer: "UCLICKER"}).Error)
	require.NoError(t, db.Create(&models.ReviewTask{ID: "t2", Repo: "owner/theirs", PRNumber: 2, Status: "in_review", Reviewer: "Uopener"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowMyReviews, "C12345", "UCLICKER"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "owner/mine")
	assert.NotContains(t, text, "owner/theirs")
}

func TestMenuActionView_UserMappingsAreListed(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.UserMapping{ID: "m1", GithubUsername: "octocat", SlackUserID: "UOCTO"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowUserMappings, "C12345", "U1"))

	require.True(t, ok)
	assert.Contains(t, viewText(t, view), "octocat")
}

func TestMenuActionView_RefusesWithoutTheMenuContext(t *testing.T) {
	db := setupTestDB(t)

	t.Run("unknown operation", func(t *testing.T) {
		_, ok := menuActionView(db, menuActionPayload(t, "no_such_op", "C12345", "U1"))
		assert.False(t, ok)
	})
	t.Run("no channel in metadata", func(t *testing.T) {
		_, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowLabels, "", "U1"))
		assert.False(t, ok)
	})
	t.Run("no view", func(t *testing.T) {
		payload := menuActionPayload(t, services.MenuOpShowLabels, "C12345", "U1")
		payload.View = nil
		_, ok := menuActionView(db, payload)
		assert.False(t, ok)
	})
}

func TestMenuAvailability_ListsOnlyUnexpiredLeaveWithARemoveButtonEach(t *testing.T) {
	db := setupTestDB(t)
	future := time.Now().Add(48 * time.Hour)
	past := time.Now().Add(-48 * time.Hour)
	require.NoError(t, db.Create(&models.ReviewerAvailability{ID: "active", SlackUserID: "UACTIVE", AwayUntil: &future}).Error)
	require.NoError(t, db.Create(&models.ReviewerAvailability{ID: "expired", SlackUserID: "UEXPIRED", AwayUntil: &past}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowAvailability, "C12345", "U1"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "UACTIVE")
	assert.Contains(t, text, `"value":"active"`)
	assert.NotContains(t, text, "UEXPIRED")
}

func TestMenuUnsetAway_RemovesOnlyTheClickedPeriod(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	first := time.Now().Add(24 * time.Hour)
	second := time.Now().Add(72 * time.Hour)
	require.NoError(t, db.Create(&models.ReviewerAvailability{ID: "leave-1", SlackUserID: "U1", AwayUntil: &first}).Error)
	require.NoError(t, db.Create(&models.ReviewerAvailability{ID: "leave-2", SlackUserID: "U1", AwayUntil: &second}).Error)

	payload := `{"type":"block_actions","trigger_id":"t","user":{"id":"U9"},
		"actions":[{"action_id":"menu_unset_away","value":"leave-1"}],"container":{"type":"view"},
		"view":{"id":"V2","callback_id":"menu_result","private_metadata":"{\"c\":\"C12345\",\"u\":\"U9\"}"}}`
	w := postPayload(t, router, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	var remaining []models.ReviewerAvailability
	require.NoError(t, db.Unscoped().Find(&remaining).Error)
	require.Len(t, remaining, 1, "the row must be hard-deleted and the user's other period kept")
	assert.Equal(t, "leave-2", remaining[0].ID)
}

func TestMenuUnsetAway_AlreadyRemovedPeriodIsNotAnError(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{"type":"block_actions","trigger_id":"t","user":{"id":"U9"},
		"actions":[{"action_id":"menu_unset_away","value":"gone"}],"container":{"type":"view"},
		"view":{"id":"V2","callback_id":"menu_result","private_metadata":"{\"c\":\"C12345\",\"u\":\"U9\"}"}}`

	assert.Equal(t, http.StatusOK, postPayload(t, router, payload).Code)
}

// settingsSubmissionWithRenameField submits the modal for label, carrying a
// rename field that was rendered for renameFieldOf.
func settingsSubmissionWithRenameField(label, renameFieldOf, renameTo string) string {
	return fmt.Sprintf(`{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"type": "static_select", "selected_option": {"value": %q}}},
					%q: {"rename": {"type": "plain_text_input", "value": %q}},
					"default_mention_user": {"default_mention_user": {"type": "users_select", "selected_user": "U777"}},
					"repository_list": {"repository_list": {"type": "plain_text_input", "value": "owner/repo"}},
					"reminder_interval": {"reminder_interval": {"type": "plain_text_input", "value": "20"}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"type": "plain_text_input", "value": "60"}},
					"business_hours_start": {"business_hours_start": {"type": "plain_text_input", "value": "10:00"}},
					"business_hours_end": {"business_hours_end": {"type": "plain_text_input", "value": "19:00"}},
					"timezone": {"timezone": {"type": "plain_text_input", "value": "UTC"}},
					"required_approvals": {"required_approvals": {"type": "plain_text_input", "value": "2"}},
					"language": {"language": {"type": "static_select", "selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"type": "static_select", "selected_option": {"value": "true"}}}
				}
			}
		}
	}`, label, services.RenameLabelBlockID(renameFieldOf), renameTo)
}

func settingsSubmission(label, renameTo string) string {
	return settingsSubmissionWithRenameField(label, label, renameTo)
}

func TestSettingsModal_RenameMovesTheExistingConfigToTheNewLabel(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "existing", SlackChannelID: "C99999", LabelName: "needs-review", ReviewerList: "UKEEP"}).Error)

	w := postPayload(t, router, settingsSubmission("needs-review", "ready-for-review"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String(), "body: %s", w.Body.String())
	var configs []models.ChannelConfig
	require.NoError(t, db.Where("slack_channel_id = ?", "C99999").Find(&configs).Error)
	require.Len(t, configs, 1, "a rename must not leave the old label behind")
	assert.Equal(t, "existing", configs[0].ID)
	assert.Equal(t, "ready-for-review", configs[0].LabelName)
	assert.Equal(t, "owner/repo", configs[0].RepositoryList, "the other fields of the same submission are still saved")
}

func TestSettingsModal_EmptyRenameKeepsTheLabelName(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "existing", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)

	w := postPayload(t, router, settingsSubmission("needs-review", ""))

	assert.Equal(t, http.StatusOK, w.Code)
	var cfg models.ChannelConfig
	require.NoError(t, db.Where("id = ?", "existing").First(&cfg).Error)
	assert.Equal(t, "needs-review", cfg.LabelName)
}

func TestSettingsModal_RenameOntoAnotherLabelIsRejected(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "a", SlackChannelID: "C99999", LabelName: "needs-review", RepositoryList: "owner/original"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "b", SlackChannelID: "C99999", LabelName: "bug"}).Error)

	w := postPayload(t, router, settingsSubmission("needs-review", "bug"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), services.RenameLabelBlockID("needs-review"))
	var cfg models.ChannelConfig
	require.NoError(t, db.Where("id = ?", "a").First(&cfg).Error)
	assert.Equal(t, "needs-review", cfg.LabelName)
	assert.Equal(t, "owner/original", cfg.RepositoryList, "a rejected submission must change nothing")
}

func TestSettingsModal_RenameFieldIsOfferedOnlyForAnExistingLabel(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "existing", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)

	editing := viewText(t, settingsModalView(db, "C99999", "U1", "needs-review"))
	creating := viewText(t, settingsModalView(db, "C99999", "U1", services.CreateNewLabelSentinel))

	assert.Contains(t, editing, services.RenameLabelBlockID("needs-review"))
	assert.NotContains(t, creating, "rename_label")
}

// Slack keeps a typed value across views.update while the block_id is the same,
// so a name typed for one label must not act on the label switched to afterwards.
func TestSettingsModal_RenameTypedForAnotherLabelIsIgnored(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "a", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "b", SlackChannelID: "C99999", LabelName: "bug"}).Error)

	w := postPayload(t, router, settingsSubmissionWithRenameField("bug", "needs-review", "renamed"))

	assert.Equal(t, http.StatusOK, w.Code)
	var cfg models.ChannelConfig
	require.NoError(t, db.Where("id = ?", "b").First(&cfg).Error)
	assert.Equal(t, "bug", cfg.LabelName)
}

func TestSettingsModal_RenameOfALabelRemovedMeanwhileIsRejected(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	w := postPayload(t, router, settingsSubmission("needs-review", "renamed"))

	assert.Contains(t, w.Body.String(), services.RenameLabelBlockID("needs-review"))
	var count int64
	require.NoError(t, db.Model(&models.ChannelConfig{}).Count(&count).Error)
	assert.Zero(t, count, "no row may be created under the new name")
}

func TestSettingsModal_RenameToTheReservedNameIsRejected(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "a", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)

	w := postPayload(t, router, settingsSubmission("needs-review", services.CreateNewLabelSentinel))

	assert.Contains(t, w.Body.String(), services.RenameLabelBlockID("needs-review"))
	var cfg models.ChannelConfig
	require.NoError(t, db.Where("id = ?", "a").First(&cfg).Error)
	assert.Equal(t, "needs-review", cfg.LabelName)
}

func TestSettingsModal_DeleteWinsOverRename(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "a", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)

	payload := strings.Replace(settingsSubmission("needs-review", "renamed"),
		`"is_active":`, `"delete_config": {"delete_config": {"type": "checkboxes", "selected_options": [{"value": "yes"}]}}, "is_active":`, 1)
	w := postPayload(t, router, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	var count int64
	require.NoError(t, db.Unscoped().Model(&models.ChannelConfig{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestHelpMenuButton_OpensTheMenuForTheChannelItWasClickedIn(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, `{"type":"block_actions","trigger_id":"trigger-open","user":{"id":"U1"},
		"actions":[{"action_id":"open_menu","value":"default"}],"container":{"channel_id":"C12345"}}`)

	require.Equal(t, http.StatusOK, w.Code)
	call := nextSlackCall(t, calls)
	assert.Equal(t, "/views.open", call.Path)
	assert.Equal(t, "trigger-open", call.Body["trigger_id"])
	view := slackCallView(t, call)
	assert.Equal(t, services.MenuModalCallbackID, view["callback_id"])
	assert.Equal(t, `{"c":"C12345","u":"U1"}`, view["private_metadata"])
}

// A menu opened without its channel would open fine and then ignore every click.
func TestHelpMenuButton_DoesNotOpenAMenuThatHasNoChannel(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, `{"type":"block_actions","trigger_id":"trigger-open","user":{"id":"U1"},
		"actions":[{"action_id":"open_menu","value":"default"}],"container":{}}`)

	require.Equal(t, http.StatusOK, w.Code)
	requireNoSlackCall(t, calls)
}

func TestMenuButton_StacksItsViewOnTopOfTheMenu(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, `{"type":"block_actions","trigger_id":"trigger-push","user":{"id":"U1"},
		"actions":[{"action_id":"menu:show_labels","value":"show_labels"}],"container":{"type":"view"},
		"view":{"id":"V1","callback_id":"menu_modal","private_metadata":"{\"c\":\"C12345\",\"u\":\"U1\"}"}}`)

	require.Equal(t, http.StatusOK, w.Code)
	call := nextSlackCall(t, calls)
	assert.Equal(t, "/views.push", call.Path, "views.open would replace the menu instead of stacking on it")
	assert.Equal(t, "trigger-push", call.Body["trigger_id"])
	assert.Equal(t, services.MenuResultCallbackID, slackCallView(t, call)["callback_id"])
}

func TestMenuUnsetAway_AnnouncesTheRemovalAndRefreshesTheList(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review", Timezone: "Asia/Tokyo"}).Error)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	from := time.Date(2099, 12, 1, 0, 0, 0, 0, tokyo)
	until := time.Date(2099, 12, 3, 23, 59, 59, 0, tokyo)
	require.NoError(t, db.Create(&models.ReviewerAvailability{ID: "leave-1", SlackUserID: "UAWAY", AwayFrom: &from, AwayUntil: &until}).Error)
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, `{"type":"block_actions","trigger_id":"t","user":{"id":"U9"},
		"actions":[{"action_id":"menu_unset_away","value":"leave-1"}],"container":{"type":"view"},
		"view":{"id":"V2","callback_id":"menu_result","private_metadata":"{\"c\":\"C12345\",\"u\":\"U9\"}"}}`)

	require.Equal(t, http.StatusOK, w.Code)
	post := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postMessage", post.Path)
	assert.Equal(t, "C12345", post.Body["channel"])
	assert.Equal(t, "<@UAWAY> の休暇（2099-12-01 ~ 2099-12-03）を解除しました", post.Body["text"])

	update := nextSlackCall(t, calls)
	assert.Equal(t, "/views.update", update.Path)
	assert.Equal(t, "V2", update.Body["view_id"])
	assert.NotContains(t, update.Raw, "UAWAY", "the refreshed list must no longer show the removed period")
}

func TestMenuUnsetAway_AnnouncesNothingWhenThePeriodWasAlreadyGone(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	calls := startFakeSlack(t)

	postSignedPayload(t, router, `{"type":"block_actions","trigger_id":"t","user":{"id":"U9"},
		"actions":[{"action_id":"menu_unset_away","value":"gone"}],"container":{"type":"view"},
		"view":{"id":"V2","callback_id":"menu_result","private_metadata":"{\"c\":\"C12345\",\"u\":\"U9\"}"}}`)

	assert.Equal(t, "/views.update", nextSlackCall(t, calls).Path)
}

func TestSettingsModal_RenameIsAnnouncedWithBothNames(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "existing", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)
	calls := startFakeSlack(t)

	postSignedPayload(t, router, settingsSubmission("needs-review", "ready-for-review"))

	post := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postMessage", post.Path)
	assert.Equal(t, "C99999", post.Body["channel"])
	assert.Equal(t, "ラベル名を「needs-review」から「ready-for-review」に変更しました。\nラベル「ready-for-review」の設定を保存しました 🎉", post.Body["text"])
}

func TestSettingsModal_SaveWithoutRenameIsAnnouncedAsBefore(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "existing", SlackChannelID: "C99999", LabelName: "needs-review"}).Error)
	calls := startFakeSlack(t)

	postSignedPayload(t, router, settingsSubmission("needs-review", ""))

	assert.Equal(t, "ラベル「needs-review」の設定を保存しました 🎉", nextSlackCall(t, calls).Body["text"])
}

func viewTitle(t *testing.T, view map[string]any) string {
	t.Helper()
	return view["title"].(map[string]any)["text"].(string)
}

func TestMenuActionView_ResultViewIsTitledAfterItsButton(t *testing.T) {
	db := setupTestDB(t)

	for _, op := range services.MenuShowOps {
		t.Run(op, func(t *testing.T) {
			view, ok := menuActionView(db, menuActionPayload(t, op, "C12345", "U1"))
			require.True(t, ok)
			assert.Equal(t, services.MenuOpLabel(op, "ja"), viewTitle(t, view))
		})
	}
}

func TestMenuActionView_FollowsTheChannelLanguage(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review", Language: "en", ReviewerList: "UREVIEWER1"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowReviewers, "C12345", "U1"))

	require.True(t, ok)
	assert.Equal(t, "Reviewers", viewTitle(t, view))
	assert.Contains(t, viewText(t, view), "Reviewer list for label")
	assert.Equal(t, "Review Bot Menu", viewTitle(t, menuModalView(db, "C12345", "U1")))
}

// The reviewer list is its own view, not the label details under another title.
func TestMenuActionView_ReviewersViewLeavesOutTheOtherSettings(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "needs-review", ReviewerList: "UREVIEWER1", RepositoryList: "owner/app"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpShowReviewers, "C12345", "U1"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "UREVIEWER1")
	assert.NotContains(t, text, "owner/app")
}

func TestMenuActionView_EditSettingsPrefersTheDefaultLabel(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "bug", DefaultMentionID: "UBUGOWNER"}).Error)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c2", SlackChannelID: "C12345", LabelName: "needs-review", DefaultMentionID: "UDEFAULTOWNER"}).Error)

	view, ok := menuActionView(db, menuActionPayload(t, services.MenuOpEditSettings, "C12345", "U1"))

	require.True(t, ok)
	text := viewText(t, view)
	assert.Contains(t, text, "UDEFAULTOWNER")
	assert.NotContains(t, text, "UBUGOWNER")
}

// "menu" is a subcommand, so a label of that name can no longer be addressed as
// the first argument.
func TestMenuCommand_TakesPrecedenceOverALabelNamedMenu(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	require.NoError(t, db.Create(&models.ChannelConfig{ID: "c1", SlackChannelID: "C12345", LabelName: "menu", RepositoryList: "owner/app"}).Error)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "menu show")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")
	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Body.String())
}
