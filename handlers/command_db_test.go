package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCommandIntegrationTestDB creates an in-memory database for integration tests
func setupCommandIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}, &models.UserMapping{}, &models.ReviewerAvailability{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func setupHTTPRequest(t *testing.T, text, channelID string) *http.Request {
	data := url.Values{}
	data.Set("command", "/slack-review-notify")
	data.Set("text", text)
	data.Set("channel_id", channelID)
	data.Set("user_id", "U12345")

	req, err := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatalf("fail to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req
}

func TestSetBusinessHoursStartCommand_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name           string
		text           string
		channelID      string
		expectedStart  string
		expectedStatus int
		expectsError   bool
	}{
		{
			name:           "Valid business hours start setting",
			text:           "needs-review set-business-hours-start 09:00",
			channelID:      "C12345",
			expectedStart:  "09:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "Business hours start setting with default label",
			text:           "set-business-hours-start 10:00",
			channelID:      "C67890",
			expectedStart:  "10:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "Invalid time format",
			text:           "needs-review set-business-hours-start 25:00",
			channelID:      "C12345",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStart, config.BusinessHoursStart)
			}
		})
	}
}

func TestSetTimezoneCommand_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name             string
		text             string
		channelID        string
		expectedTimezone string
		expectedStatus   int
		expectsError     bool
	}{
		{
			name:             "Valid timezone setting (JST)",
			text:             "needs-review set-timezone Asia/Tokyo",
			channelID:        "C12345",
			expectedTimezone: "Asia/Tokyo",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:             "Valid timezone setting (UTC)",
			text:             "needs-review set-timezone UTC",
			channelID:        "C67890",
			expectedTimezone: "UTC",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:           "Invalid timezone",
			text:           "needs-review set-timezone Invalid/Timezone",
			channelID:      "C12345",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTimezone, config.Timezone)
			}
		})
	}
}

func TestSetRequiredApprovals_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name              string
		text              string
		channelID         string
		expectedApprovals int
		expectedStatus    int
		expectsError      bool
	}{
		{
			name:              "Valid required approvals setting",
			text:              "needs-review set-required-approvals 2",
			channelID:         "C_APPROVALS",
			expectedApprovals: 2,
			expectedStatus:    200,
			expectsError:      false,
		},
		{
			name:              "Required approvals setting with default label",
			text:              "set-required-approvals 3",
			channelID:         "C_APPROVALS2",
			expectedApprovals: 3,
			expectedStatus:    200,
			expectsError:      false,
		},
		{
			name:           "Out of range value (0)",
			text:           "needs-review set-required-approvals 0",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
		{
			name:           "Out of range value (11)",
			text:           "needs-review set-required-approvals 11",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
		{
			name:           "Invalid value (string)",
			text:           "needs-review set-required-approvals abc",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedApprovals, config.RequiredApprovals)
			}

			if tt.expectsError {
				body := w.Body.String()
				assert.Contains(t, body, "1〜10の整数")
			}
		})
	}
}

func TestSetAway_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name           string
		text           string
		channelID      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Indefinite leave setting",
			text:           "set-away <@U12345>",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "休暇に設定しました",
		},
		{
			name:           "Leave setting with date",
			text:           "set-away <@U67890> until 2099-12-31 reason 休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-12-31 まで",
		},
		{
			name:           "Leave setting with reason only",
			text:           "set-away <@U11111> reason 育児休業",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "育児休業",
		},
		{
			name:           "Leave setting with from and until",
			text:           "set-away <@U22222> from 2099-11-01 until 2099-12-31 reason 長期休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-11-01 ~ 2099-12-31",
		},
		{
			name:           "Leave setting with on (single day)",
			text:           "set-away <@U33333> on 2099-06-15 reason 有給休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-06-15",
		},
		{
			name:           "Past date is rejected",
			text:           "set-away <@U99999> until 2020-01-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "過去の日付は指定できません",
		},
		{
			name:           "Past from date is rejected",
			text:           "set-away <@U99999> from 2020-01-01 until 2099-12-31",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "過去の日付は指定できません",
		},
		{
			name:           "from after until is rejected",
			text:           "set-away <@U99999> from 2099-12-31 until 2099-01-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "開始日（from）は終了日（until）より前に指定してください",
		},
		{
			name:           "on combined with from is rejected",
			text:           "set-away <@U99999> on 2099-06-01 from 2099-05-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "同時に使えません",
		},
		{
			name:           "from without date value is rejected",
			text:           "set-away <@U99999> from",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "キーワードの後に日付を指定してください",
		},
		{
			name:           "No user specified",
			text:           "set-away",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "休暇に設定するユーザーを指定してください",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}

	// A single set-away creates exactly 1 record
	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "U12345").Count(&count)
	assert.Equal(t, int64(1), count)
}

// TestSetAway_MultiplePeriods verifies that a user can hold multiple distinct leave
// periods at once (e.g. a pre-booked vacation plus a sudden sick day), and that
// re-registering an identical period stays idempotent instead of adding a duplicate.
func TestSetAway_MultiplePeriods(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) *httptest.ResponseRecorder {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Register two separate scheduled leaves for the same user.
	assert.Equal(t, 200, send("set-away <@UMULTI> on 2099-06-01").Code)
	assert.Equal(t, 200, send("set-away <@UMULTI> on 2099-06-10").Code)

	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UMULTI").Count(&count)
	assert.Equal(t, int64(2), count, "two distinct periods should be stored separately")

	// Registering an identical period again only updates the reason; no new row.
	assert.Equal(t, 200, send("set-away <@UMULTI> on 2099-06-10 reason 再登録").Code)
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UMULTI").Count(&count)
	assert.Equal(t, int64(2), count, "re-registering the same period must not add a row")
}

// TestUnsetAway_SpecificPeriod verifies that unset-away with a date removes only the
// matching period, while unset-away without a date removes all of the user's leaves.
func TestUnsetAway_SpecificPeriod(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) *httptest.ResponseRecorder {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	send("set-away <@UPART> on 2099-06-01")
	send("set-away <@UPART> on 2099-06-10")

	// Remove only the 2099-06-01 leave.
	w := send("unset-away <@UPART> on 2099-06-01")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇を解除しました")

	var records []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "UPART").Find(&records)
	assert.Len(t, records, 1, "only the matching period should be removed")
	if len(records) == 1 && assert.NotNil(t, records[0].AwayFrom) {
		assert.Equal(t, "2099-06-10", records[0].AwayFrom.Format("2006-01-02"), "the other period must remain")
	}

	// unset-away without a date removes all remaining leaves.
	w2 := send("unset-away <@UPART>")
	assert.Equal(t, 200, w2.Code)
	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UPART").Count(&count)
	assert.Equal(t, int64(0), count)
}

// TestShowAvailability_AfterUnsetOneOfTwo verifies the display output of
// show-availability for the core bug scenario: register two leave periods for
// the same user, remove one, and confirm the other still appears in the list.
func TestShowAvailability_AfterUnsetOneOfTwo(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) string {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Body.String()
	}

	send("set-away <@UKEEP> on 2099-06-01")
	send("set-away <@UKEEP> on 2099-06-10")

	// Before removal both periods must be listed.
	before := send("show-availability")
	assert.Contains(t, before, "2099-06-01", "the first period must be listed before unset")
	assert.Contains(t, before, "2099-06-10", "the second period must be listed before unset")

	send("unset-away <@UKEEP> on 2099-06-01")

	// After removing only 2099-06-01 the 2099-06-10 period must remain visible.
	after := send("show-availability")
	assert.NotContains(t, after, "2099-06-01", "the removed period must disappear from the list")
	assert.Contains(t, after, "2099-06-10", "the surviving period must still be listed")
	assert.Contains(t, after, "UKEEP", "the user must still appear for the surviving period")
}

// TestUnsetAway_FromUntilPeriod verifies individual removal of a range period
// registered with "from/until", which produces different timestamps than "on".
func TestUnsetAway_FromUntilPeriod(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) *httptest.ResponseRecorder {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Two range periods plus an open-ended "from"-only period for the same user.
	send("set-away <@URANGE> from 2099-06-01 until 2099-06-05")
	send("set-away <@URANGE> from 2099-07-01 until 2099-07-05")
	send("set-away <@URANGE> from 2099-08-01")

	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "URANGE").Count(&count)
	assert.Equal(t, int64(3), count)

	// Remove only the first range.
	w := send("unset-away <@URANGE> from 2099-06-01 until 2099-06-05")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇を解除しました")

	var remaining []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "URANGE").Order("away_from").Find(&remaining)
	assert.Len(t, remaining, 2, "only the matching range should be removed")
	if assert.Len(t, remaining, 2) {
		assert.NotNil(t, remaining[0].AwayFrom)
		assert.Equal(t, "2099-07-01", remaining[0].AwayFrom.Format("2006-01-02"), "the 07 range must remain")
		assert.NotNil(t, remaining[1].AwayFrom)
		assert.Equal(t, "2099-08-01", remaining[1].AwayFrom.Format("2006-01-02"), "the from-only period must remain")
	}

	// Remove only the open-ended "from"-only period.
	w2 := send("unset-away <@URANGE> from 2099-08-01")
	assert.Equal(t, 200, w2.Code)
	var survivors []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "URANGE").Find(&survivors)
	assert.Len(t, survivors, 1, "the from-only period should be removable individually")
	if assert.Len(t, survivors, 1) && assert.NotNil(t, survivors[0].AwayFrom) {
		assert.Equal(t, "2099-07-01", survivors[0].AwayFrom.Format("2006-01-02"), "the 07 range must be the survivor")
	}
}

// TestUnsetAway_NoMatchKeepsPeriods verifies that a date that does not exactly
// match any stored period removes nothing and reports "not set", leaving the
// existing periods intact.
func TestUnsetAway_NoMatchKeepsPeriods(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) *httptest.ResponseRecorder {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	send("set-away <@UNOMATCH> from 2099-06-01 until 2099-06-05")

	// "on 2099-06-01" sets until=end-of-day, which does not match the stored
	// until=2099-06-05, so nothing should be deleted.
	w := send("unset-away <@UNOMATCH> on 2099-06-01")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇に設定されていません")

	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UNOMATCH").Count(&count)
	assert.Equal(t, int64(1), count, "a non-matching date must not delete any period")
}

func TestUnsetAway_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	// First, set the leave status
	gin.SetMode(gin.TestMode)
	req := setupHTTPRequest(t, "set-away <@UAWAY>", "C12345")
	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Remove leave status
	req2 := setupHTTPRequest(t, "unset-away <@UAWAY>", "C12345")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	assert.Contains(t, w2.Body.String(), "休暇を解除しました")

	// Verify no record exists in the DB
	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UAWAY").Count(&count)
	assert.Equal(t, int64(0), count)

	// Removing leave for a non-existent user
	req3 := setupHTTPRequest(t, "unset-away <@UNOTEXIST>", "C12345")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, 200, w3.Code)
	assert.Contains(t, w3.Body.String(), "休暇に設定されていません")
}

// TestUnsetAway_LegacyPipeFormat verifies that a record stored with the legacy
// "ID|displayname" slack_user_id (written by an older cleanUserID before the
// pipe-strip fix) is removed by unset-away via the LIKE fallback, exercised
// through the slash-command handler rather than only the E2E layer.
func TestUnsetAway_LegacyPipeFormat(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	// Seed a legacy-format record directly.
	require := db.Create(&models.ReviewerAvailability{
		ID:          "rec-legacy-pipe",
		SlackUserID: "ULEGACY|username",
		Reason:      "old",
	})
	assert.NoError(t, require.Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	// cleanUserID(<@ULEGACY|username>) -> "ULEGACY", a well-formed id, so the
	// LIKE fallback applies and matches the legacy "ULEGACY|username" row.
	req := setupHTTPRequest(t, "unset-away <@ULEGACY|username>", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇を解除しました")

	var count int64
	db.Unscoped().Model(&models.ReviewerAvailability{}).
		Where("slack_user_id LIKE ?", "ULEGACY%").Count(&count)
	assert.Equal(t, int64(0), count, "legacy record must be fully removed")
}

// TestUnsetAway_WildcardDoesNotMatchLegacy guards the LIKE-escaping fix: a
// caller-supplied LIKE metacharacter ("%") must NOT widen the delete to other
// users' legacy rows. cleanUserID("U%") is not a well-formed Slack id, so the
// LIKE fallback is skipped and only an exact match is attempted.
func TestUnsetAway_WildcardDoesNotMatchLegacy(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	// A legacy row that a naive `LIKE 'U%|%'` would have matched.
	res := db.Create(&models.ReviewerAvailability{
		ID:          "rec-victim",
		SlackUserID: "UVICTIM|username",
		Reason:      "should survive",
	})
	assert.NoError(t, res.Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	// "U%" must not delete UVICTIM's legacy record.
	req := setupHTTPRequest(t, "unset-away U%", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var count int64
	db.Unscoped().Model(&models.ReviewerAvailability{}).
		Where("slack_user_id = ?", "UVICTIM|username").Count(&count)
	assert.Equal(t, int64(1), count, "wildcard input must not delete another user's legacy row")
}

// TestUnsetAway_LegacyPipeFormat_SpecificPeriod guards the trickiest path: a
// legacy "ID|displayname" record removed by unset-away WITH a date. The query
// combines "(exact OR LIKE) AND period", so the period predicate must apply to
// the LIKE branch too — only the matching period of the legacy row is removed,
// and a non-matching legacy period for the same user survives.
func TestUnsetAway_LegacyPipeFormat_SpecificPeriod(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) *httptest.ResponseRecorder {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Seed two periods via set-away so the stored away_from/away_until exactly
	// match what the parser produces, then rewrite the id to the legacy pipe
	// format an older cleanUserID would have stored.
	send("set-away <@UPIPEP> on 2099-06-01")
	send("set-away <@UPIPEP> on 2099-06-10")
	assert.NoError(t, db.Model(&models.ReviewerAvailability{}).
		Where("slack_user_id = ?", "UPIPEP").
		Update("slack_user_id", "UPIPEP|username").Error)

	// Remove only the 2099-06-01 period of the legacy-format records.
	w := send("unset-away <@UPIPEP|username> on 2099-06-01")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇を解除しました")

	var records []models.ReviewerAvailability
	db.Where("slack_user_id LIKE ?", "UPIPEP%").Find(&records)
	assert.Len(t, records, 1, "only the matching legacy period should be removed")
	if len(records) == 1 && assert.NotNil(t, records[0].AwayFrom) {
		assert.Equal(t, "2099-06-10", records[0].AwayFrom.Format("2006-01-02"),
			"the non-matching legacy period must remain")
	}
}

func TestShowAvailability_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	// When no one is on leave
	req := setupHTTPRequest(t, "show-availability", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇中・予約中のユーザーはいません")

	// Add users on leave
	req2 := setupHTTPRequest(t, "set-away <@USHOW1> reason 休暇", "C12345")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)

	req3 := setupHTTPRequest(t, "set-away <@USHOW2> until 2099-12-31 reason 育児休業", "C12345")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, 200, w3.Code)

	// Add a scheduled (future) leave
	req3b := setupHTTPRequest(t, "set-away <@USHOW3> from 2099-06-01 until 2099-06-15 reason 予定休暇", "C12345")
	w3b := httptest.NewRecorder()
	router.ServeHTTP(w3b, req3b)
	assert.Equal(t, 200, w3b.Code)

	// Verify the leave list
	req4 := setupHTTPRequest(t, "show-availability", "C12345")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	assert.Equal(t, 200, w4.Code)
	body := w4.Body.String()
	assert.Contains(t, body, "休暇中・予約中のユーザー")
	assert.Contains(t, body, "USHOW1")
	assert.Contains(t, body, "USHOW2")
	assert.Contains(t, body, "休暇中")   // status label for currently away
	assert.Contains(t, body, "予約中")   // status label for scheduled
	assert.Contains(t, body, "USHOW3")
	assert.Contains(t, body, "予定休暇")
}

// TestMapUserCommand_RejectsPlainHandle ensures that `map-user` refuses to
// persist a Slack identifier that isn't a resolved user id. Without this
// guard, a plain @handle is stored verbatim and silently mismatches the
// reviewer_list rewritten by the modal picker (which always emits U-ids), so
// the PR author ends up in the candidate pool.
func TestMapUserCommand_RejectsPlainHandle(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "map-user octocat @octocat", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "SlackユーザーIDが解決できませんでした")

	var count int64
	db.Model(&models.UserMapping{}).Where("github_username = ?", "octocat").Count(&count)
	assert.Equal(t, int64(0), count, "non-user-id input must not be persisted")
}

// TestMapUserCommand_AcceptsResolvedUserID verifies that the canonical input
// shape — a Slack-resolved `<@U…>` mention or a bare U-id — still works.
func TestMapUserCommand_AcceptsResolvedUserID(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "map-user octocat <@U01ABCDE234>", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var mapping models.UserMapping
	if err := db.Where("github_username = ?", "octocat").First(&mapping).Error; err != nil {
		t.Fatalf("expected user mapping to be created, got: %v", err)
	}
	assert.Equal(t, "U01ABCDE234", mapping.SlackUserID)
}

// TestMapUserCommand_AcceptsEscapedMentionWithName confirms that the autocomplete
// form `<@U…|displayname>` is normalized down to the bare user id.
func TestMapUserCommand_AcceptsEscapedMentionWithName(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "map-user octocat <@U01ABCDE234|octocat>", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var mapping models.UserMapping
	if err := db.Where("github_username = ?", "octocat").First(&mapping).Error; err != nil {
		t.Fatalf("expected user mapping to be created, got: %v", err)
	}
	assert.Equal(t, "U01ABCDE234", mapping.SlackUserID)
}
