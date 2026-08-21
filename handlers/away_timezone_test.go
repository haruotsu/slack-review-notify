package handlers

import (
	"net/http/httptest"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestUnsetAway_OnDate_DSTMidnightGap pins the day window used by
// "unset-away @user on DATE" against a timezone where local midnight does not
// exist. Asia/Beirut springs 00:00 -> 01:00 on 2027-03-28, so time.Date
// normalizes the day start to 01:00; deriving the end boundary with
// AddDate(0,0,1) carried that wall clock forward and stretched the window an
// hour into 2027-03-29, hard-deleting a leave registered there.
func TestUnsetAway_OnDate_DSTMidnightGap(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	assert.NoError(t, db.Create(&models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: "C12345",
		LabelName:      defaultLabelName,
		Timezone:       "Asia/Beirut",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	send := func(text string) string {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Body.String()
	}

	send("set-away <@UBEI> on 2027-03-28 reason target-day")
	send("set-away <@UBEI> on 2027-03-29 00:00-00:30 reason next-day-early")

	var before int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UBEI").Count(&before)
	assert.Equal(t, int64(2), before, "precondition: both leaves registered")

	send("unset-away <@UBEI> on 2027-03-28")

	var remaining []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "UBEI").Find(&remaining)
	assert.Len(t, remaining, 1, "only the 2027-03-28 leave may be deleted")
	if len(remaining) == 1 {
		assert.Equal(t, "next-day-early", remaining[0].Reason,
			"the 2027-03-29 00:00-00:30 leave must survive")
	}
}

// TestAwayModal_TimezoneMatchesShowAvailability pins that the zone the modal
// writes in and the zone show-availability renders in are the same function of
// the channel. They used to differ: the modal took the first label with a
// timezone (label_name order) while the listing resolved the invoked label, so
// a full-day leave registered through the modal was listed as a cross-day time
// range in a channel whose labels carry different timezones.
func TestAwayModal_TimezoneMatchesShowAvailability(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	// "aaa-label" sorts before the default label, so the old first-match rule
	// would have picked America/New_York here.
	for _, c := range []struct{ label, tz string }{
		{"aaa-label", "America/New_York"},
		{defaultLabelName, "Asia/Tokyo"},
	} {
		assert.NoError(t, db.Create(&models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: "C12345",
			LabelName:      c.label,
			Timezone:       c.tz,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}).Error)
	}

	modalLoc := resolveAwayTimezone(db, "C12345")
	listingLoc := resolveTimezone(db, "C12345", defaultLabelName)
	assert.Equal(t, listingLoc.String(), modalLoc.String(),
		"the modal must write in the zone show-availability renders in")

	// End to end: a full-day leave submitted through the modal must be listed
	// as a plain date, not as a time range spilling into the next day.
	values := map[string]map[string]services.ViewStateValue{
		"away_user":   {"away_user": {SelectedUser: "UMODALTZ"}},
		"away_from":   {"away_from": {SelectedDate: "2099-05-01"}},
		"away_until":  {"away_until": {SelectedDate: "2099-05-01"}},
		"away_reason": {"away_reason": {Value: "全休"}},
	}
	form, err := services.ParseAwayModalSubmission(values, modalLoc, "ja")
	assert.NoError(t, err)
	assert.NoError(t, db.Create(&models.ReviewerAvailability{
		ID:          uuid.NewString(),
		SlackUserID: form.SlackUserID,
		AwayFrom:    form.AwayFrom,
		AwayUntil:   form.AwayUntil,
		Reason:      form.Reason,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error)

	req := setupHTTPRequest(t, "show-availability", "C12345")
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))
	router.ServeHTTP(w, req)

	out := w.Body.String()
	assert.Contains(t, out, "2099-05-01", "the registered day must be listed")
	assert.NotContains(t, out, "2099-05-02", "a full-day leave must not spill into the next day")
	assert.NotContains(t, out, "13:00", "a full-day leave must not be listed as a time range")
}

// TestAwayTimezone_SameForEveryLabel pins that the leave feature resolves one
// timezone per channel, whichever label the caller routes the command through.
//
// reviewer_availabilities has no label_name column, so reading a record through
// a second label's timezone renders something nobody registered: with the zone
// resolved per label, a full day written under one label was listed as
// "2099-04-30 17:00 ~ 2099-05-01 16:59" under another. Both writers must land on
// the zone the listing uses, and the listing must not change with the prefix.
func TestAwayTimezone_SameForEveryLabel(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	for _, c := range []struct{ label, tz string }{
		{defaultLabelName, "Asia/Tokyo"},
		{"eu-team", "Europe/Berlin"},
		{"aaa-label", "America/New_York"},
	} {
		assert.NoError(t, db.Create(&models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: "C12345",
			LabelName:      c.label,
			Timezone:       c.tz,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}).Error)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))
	send := func(text string) string {
		req := setupHTTPRequest(t, text, "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Body.String()
	}

	// Written through an explicit non-default label...
	send("eu-team set-away <@ULABEL> on 2099-05-01 reason 全休")

	// ...and through the modal.
	modalLoc := resolveAwayTimezone(db, "C12345")
	form, err := services.ParseAwayModalSubmission(map[string]map[string]services.ViewStateValue{
		"away_user":  {"away_user": {SelectedUser: "UMODAL"}},
		"away_from":  {"away_from": {SelectedDate: "2099-05-01"}},
		"away_until": {"away_until": {SelectedDate: "2099-05-01"}},
	}, modalLoc, "ja")
	assert.NoError(t, err)
	assert.NoError(t, db.Create(&models.ReviewerAvailability{
		ID:          uuid.NewString(),
		SlackUserID: form.SlackUserID,
		AwayFrom:    form.AwayFrom,
		AwayUntil:   form.AwayUntil,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error)

	// Both records must read back as a plain full day, under every label prefix.
	for _, cmd := range []string{
		"show-availability",
		"eu-team show-availability",
		"aaa-label show-availability",
	} {
		out := send(cmd)
		assert.Contains(t, out, "ULABEL", "%q must list the slash-command record", cmd)
		assert.Contains(t, out, "UMODAL", "%q must list the modal record", cmd)
		assert.Contains(t, out, "2099-05-01", "%q must show the registered day", cmd)
		assert.NotContains(t, out, "2099-04-30", "%q must not shift a full day backwards", cmd)
		assert.NotContains(t, out, "2099-05-02", "%q must not spill a full day forwards", cmd)
		assert.NotContains(t, out, ":00-", "%q must not turn a full day into a time range", cmd)
	}
}

// TestAwayTimezone_FallsBackToTheChannelsOwnZone pins that resolving per
// channel does not throw away the timezone a channel actually configured.
//
// A channel that has only ever used one label has no default-label row, and
// resolving the default label alone drops straight to Asia/Tokyo there. That is
// worse than the per-label resolution it replaced: the period is stored in the
// wrong zone, so GetAwayUserIDs excludes the reviewer over the wrong hours while
// show-availability renders the times the caller typed, hiding the shift.
func TestAwayTimezone_FallsBackToTheChannelsOwnZone(t *testing.T) {
	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	berlin, err := time.LoadLocation("Europe/Berlin")
	assert.NoError(t, err)

	cases := []struct {
		name string
		// blankDefaultLabel adds a default-label row and then clears its
		// timezone with a raw UPDATE. ChannelConfig.Timezone carries
		// `default:'Asia/Tokyo'`, so a blank value cannot be written through
		// Create — but rows predating that column can still hold one.
		blankDefaultLabel bool
	}{
		{name: "default label row missing"},
		{name: "default label row has a blank timezone", blankDefaultLabel: true},
	}

	for _, tc := range cases {
		db := setupCommandIntegrationTestDB(t)
		assert.NoError(t, db.Create(&models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: "C12345",
			LabelName:      "eu-team",
			Timezone:       "Europe/Berlin",
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}).Error)
		if tc.blankDefaultLabel {
			assert.NoError(t, db.Create(&models.ChannelConfig{
				ID:             uuid.NewString(),
				SlackChannelID: "C12345",
				LabelName:      defaultLabelName,
				IsActive:       true,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}).Error)
			assert.NoError(t, db.Model(&models.ChannelConfig{}).
				Where("slack_channel_id = ? AND label_name = ?", "C12345", defaultLabelName).
				Update("timezone", "").Error)
		}

		assert.Equal(t, "Europe/Berlin", resolveAwayTimezone(db, "C12345").String(),
			"%s: the channel's only configured zone must win over the default", tc.name)

		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/slack/command", HandleSlackCommand(db))
		req := setupHTTPRequest(t, "eu-team set-away <@UEU> on 2099-05-01 09:00-17:00", "C12345")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var rec models.ReviewerAvailability
		assert.NoError(t, db.Where("slack_user_id = ?", "UEU").First(&rec).Error)
		if assert.NotNil(t, rec.AwayFrom) && assert.NotNil(t, rec.AwayUntil) {
			assert.Equal(t, 9, rec.AwayFrom.In(berlin).Hour(), "%s: start must be 09:00 Berlin", tc.name)
			assert.Equal(t, 17, rec.AwayUntil.In(berlin).Hour(), "%s: end must be 17:00 Berlin", tc.name)
		}
	}
}

// TestShowAvailability_QueryBindIsUTC pins the "not expired yet" filter in
// show-availability. The cutoff is time.Now() in the *process* timezone, so
// binding it unconverted shifts the comparison by the process offset and drops
// leave that is still running. time.Local is forced to a non-UTC zone because
// the bug is invisible when the process already runs in UTC (as CI does).
func TestShowAvailability_QueryBindIsUTC(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)
	originalLocal := time.Local
	time.Local = jst
	defer func() { time.Local = originalLocal }()

	// Ends 30 minutes from now: inside the window a +09:00 bind would skip over,
	// so an unconverted cutoff makes this row vanish from the listing.
	endsSoon := time.Now().Add(30 * time.Minute)
	assert.NoError(t, db.Create(&models.ReviewerAvailability{
		ID:          uuid.NewString(),
		SlackUserID: "UENDSOON",
		AwayUntil:   &endsSoon,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))
	req := setupHTTPRequest(t, "show-availability", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Contains(t, w.Body.String(), "UENDSOON",
		"leave that has not ended yet must be listed regardless of the process timezone")
}
