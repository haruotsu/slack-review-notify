package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type helpBlock struct {
	Type string `json:"type"`
	Text *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"text"`
	Elements []struct {
		Type     string `json:"type"`
		ActionID string `json:"action_id"`
		Text     *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"text"`
	} `json:"elements"`
}

type helpResponse struct {
	ResponseType string      `json:"response_type"`
	Text         string      `json:"text"`
	Blocks       []helpBlock `json:"blocks"`
}

func postHelpCommand(t *testing.T, db *gorm.DB, channelID string) helpResponse {
	t.Helper()

	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "help")
	form.Add("channel_id", channelID)
	form.Add("user_id", "U12345")

	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)

	var resp helpResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// TestHelpCommand_SectionTextWithinSlackLimit asserts that no section block of the
// help response exceeds Slack's per-section text limit. One oversized section makes
// Slack reject the entire slash command response with invalid_command_response.
func TestHelpCommand_SectionTextWithinSlackLimit(t *testing.T) {
	for _, lang := range []string{"ja", "en"} {
		t.Run(lang, func(t *testing.T) {
			db := setupTestDB(t)
			require.NoError(t, db.Create(&models.ChannelConfig{
				ID:             "config-" + lang,
				SlackChannelID: "C12345",
				LabelName:      "needs-review",
				Language:       lang,
			}).Error)

			resp := postHelpCommand(t, db, "C12345")

			sectionCount := 0
			for i, block := range resp.Blocks {
				if block.Type != "section" || block.Text == nil {
					continue
				}
				sectionCount++
				assert.LessOrEqual(t, utf8.RuneCountInString(block.Text.Text), services.SlackSectionTextLimit,
					"blocks[%d] exceeds the Slack section text limit", i)
			}
			assert.Positive(t, sectionCount, "help response must contain at least one section block")
			assert.LessOrEqual(t, len(resp.Blocks), services.SlackMaxBlocks)
		})
	}
}

// TestHelpCommand_SectionTextKeepsWholeHelpBody asserts that splitting the help body
// across section blocks loses nothing: rejoining the sections reproduces it verbatim.
func TestHelpCommand_SectionTextKeepsWholeHelpBody(t *testing.T) {
	for _, lang := range []string{"ja", "en"} {
		t.Run(lang, func(t *testing.T) {
			db := setupTestDB(t)
			require.NoError(t, db.Create(&models.ChannelConfig{
				ID:             "config-" + lang,
				SlackChannelID: "C12345",
				LabelName:      "needs-review",
				Language:       lang,
			}).Error)

			resp := postHelpCommand(t, db, "C12345")

			sections := []string{}
			for _, block := range resp.Blocks {
				if block.Type == "section" && block.Text != nil {
					sections = append(sections, block.Text.Text)
				}
			}

			assert.Equal(t, i18n.TWithLang(lang, "cmd.help"), strings.Join(sections, "\n"))
		})
	}
}

// TestHelpCommand_ButtonTextWithinSlackLimit asserts that a long label name cannot push
// a button label past Slack's plain_text limit for buttons, which would be rejected the
// same way an oversized section is.
func TestHelpCommand_ButtonTextWithinSlackLimit(t *testing.T) {
	db := setupTestDB(t)
	longLabel := strings.Repeat("very-long-label-name,", 6) + "needs-review"
	require.NoError(t, db.Create(&models.ChannelConfig{
		ID:             "config-long",
		SlackChannelID: "C12345",
		LabelName:      longLabel,
		Language:       "ja",
	}).Error)

	resp := postHelpCommand(t, db, "C12345")

	buttonCount := 0
	for i, block := range resp.Blocks {
		for j, element := range block.Elements {
			if element.Type != "button" || element.Text == nil {
				continue
			}
			buttonCount++
			assert.LessOrEqual(t, utf8.RuneCountInString(element.Text.Text), services.SlackButtonTextLimit,
				"blocks[%d].elements[%d] exceeds the Slack button text limit", i, j)
		}
	}
	assert.Positive(t, buttonCount, "help response must contain buttons")
}

// TestHelpText_DocumentsEverySubCommand keeps the help text honest about what the bot
// accepts: a subcommand the help never mentions is a subcommand nobody discovers.
func TestHelpText_DocumentsEverySubCommand(t *testing.T) {
	for _, lang := range []string{"ja", "en"} {
		t.Run(lang, func(t *testing.T) {
			help := i18n.TWithLang(lang, "cmd.help")
			for _, sub := range subCommands {
				assert.Contains(t, help, sub, "help text must document the %q subcommand", sub)
			}
		})
	}
}
