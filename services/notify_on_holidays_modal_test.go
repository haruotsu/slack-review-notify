package services

import (
	"slack-review-notify/models"
	"testing"
)

func notifyOnHolidaysElement(t *testing.T, view map[string]any) map[string]any {
	t.Helper()
	for _, b := range view["blocks"].([]map[string]any) {
		if id, _ := b["block_id"].(string); id == "notify_on_holidays" {
			el, ok := b["element"].(map[string]any)
			if !ok {
				t.Fatalf("notify_on_holidays block has no element")
			}
			return el
		}
	}
	t.Fatalf("notify_on_holidays block not found")
	return nil
}

func initialOptionValue(el map[string]any) string {
	opt, _ := el["initial_option"].(map[string]any)
	v, _ := opt["value"].(string)
	return v
}

func TestBuildSettingsModalView_NotifyOnHolidaysReflectsConfig(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
		want    string
	}{
		{"有効なラベルでは通知する側が選ばれている", true, "true"},
		{"無効なラベルでは通知しない側が選ばれている", false, "false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &models.ChannelConfig{
				SlackChannelID:   "C12345",
				LabelName:        "needs-review",
				NotifyOnHolidays: tt.enabled,
				IsActive:         true,
			}
			view := BuildSettingsModalView(SettingsModalInputs{
				ChannelID:     "C12345",
				UserID:        "U1",
				SelectedLabel: "needs-review",
				Configs:       []*models.ChannelConfig{cfg},
				Lang:          "ja",
			})

			if got := initialOptionValue(notifyOnHolidaysElement(t, view)); got != tt.want {
				t.Errorf("notify_on_holidays.initial_option.value = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSettingsModalView_NotifyOnHolidaysDefaultsToOffForNewLabel(t *testing.T) {
	view := BuildSettingsModalView(SettingsModalInputs{
		ChannelID:     "C12345",
		UserID:        "U1",
		SelectedLabel: CreateNewLabelSentinel,
		Lang:          "ja",
	})

	if got := initialOptionValue(notifyOnHolidaysElement(t, view)); got != "false" {
		t.Errorf("notify_on_holidays.initial_option.value = %q, want false", got)
	}
}

func TestParseSettingsModalSubmission_NotifyOnHolidays(t *testing.T) {
	v := minimalValidParseValues()
	v["notify_on_holidays"] = map[string]ViewStateValue{
		"notify_on_holidays": {SelectedOption: &ViewSelectedOption{Value: "true"}},
	}

	got, err := ParseSettingsModalSubmission(v)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.NotifyOnHolidays {
		t.Errorf("NotifyOnHolidays = false, want true")
	}
}

func TestParseSettingsModalSubmission_NotifyOnHolidaysMissingMeansOff(t *testing.T) {
	got, err := ParseSettingsModalSubmission(minimalValidParseValues())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.NotifyOnHolidays {
		t.Errorf("NotifyOnHolidays = true, want false")
	}
}

func TestParseSettingsModalSubmission_NotifyOnHolidaysRejectsUnknownValue(t *testing.T) {
	v := minimalValidParseValues()
	v["notify_on_holidays"] = map[string]ViewStateValue{
		"notify_on_holidays": {SelectedOption: &ViewSelectedOption{Value: "maybe"}},
	}

	_, err := ParseSettingsModalSubmission(v)
	verr, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("err = %v, want *ModalValidationError", err)
	}
	if _, ok := verr.Errors["notify_on_holidays"]; !ok {
		t.Errorf("errors = %v, want notify_on_holidays key", verr.Errors)
	}
}
