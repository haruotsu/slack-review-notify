package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"strconv"
	"strings"
	"time"

	"regexp"

	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HandleSlackCommand is a handler that processes Slack slash commands
func HandleSlackCommand(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		// Restore the body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Verify the signature
		if !services.ValidateSlackRequest(c.Request, bodyBytes) {
			log.Println("invalid slack signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid slack signature"})
			return
		}

		command := c.PostForm("command")
		text := c.PostForm("text")
		channelID := c.PostForm("channel_id")
		userID := c.PostForm("user_id")

		log.Printf("slack command received: command=%s, text=%s, channel=%s, user=%s",
			command, text, channelID, userID)

		// Output all channel configs for debugging
		var allConfigs []models.ChannelConfig
		db.Find(&allConfigs)
		log.Printf("all channel configs in database (%d):", len(allConfigs))
		for i, cfg := range allConfigs {
			log.Printf("[%d] ID=%s, Channel=%s, Label=%s", i, cfg.ID, cfg.SlackChannelID, cfg.LabelName)
		}

		// Process the /slack-review-notify command
		if command == "/slack-review-notify" {
			// Separate the command part from parameters
			var labelName, subCommand, params string

			// Split text with quote support
			parts := parseCommand(text)

			if len(parts) == 0 {
				// Show help when no arguments are provided
				var emptyHelpConfig models.ChannelConfig
				emptyHelpLang := "ja"
				if err := db.Where("slack_channel_id = ?", channelID).First(&emptyHelpConfig).Error; err == nil {
					emptyHelpLang = getLang(&emptyHelpConfig)
				}
				showHelp(c, emptyHelpLang)
				return
			}

			// Determine whether the first argument is a subcommand or a label name
			potentialSubCommands := []string{"show", "help", "set-mention", "add-reviewer",
				"show-reviewers", "clear-reviewers", "add-repo", "remove-repo",
				"set-label", "activate", "deactivate", "set-reviewer-reminder-interval",
				"set-business-hours-start", "set-business-hours-end", "set-timezone",
				"map-user", "show-user-mappings", "remove-user-mapping",
				"set-required-approvals", "set-language",
				"set-away", "unset-away", "show-availability"}

			isSubCommand := false
			for _, cmd := range potentialSubCommands {
				if parts[0] == cmd {
					isSubCommand = true
					break
				}
			}

			if isSubCommand {
				// If the first argument is a subcommand, use the default label name
				subCommand = parts[0]
				labelName = "needs-review" // Default label name

				if len(parts) > 1 {
					params = strings.Join(parts[1:], " ")
				}
			} else {
				// If the first argument is a label name
				labelName = parts[0]

				if len(parts) > 1 {
					subCommand = parts[1]

					if len(parts) > 2 {
						params = strings.Join(parts[2:], " ")
					}
				} else {
					// If only the label name is specified, show its settings
					subCommand = "show"
				}
			}

			// Log information about the label name
			log.Printf("command parsed: label=%s, subCommand=%s, params=%s",
				labelName, subCommand, params)

			if subCommand == "" || subCommand == "help" {
				// Show help - try to get lang from any config for this channel
				var helpConfig models.ChannelConfig
				helpLang := "ja"
				if err := db.Where("slack_channel_id = ?", channelID).First(&helpConfig).Error; err == nil {
					helpLang = getLang(&helpConfig)
				}
				showHelp(c, helpLang)
				return
			}

			// Get existing config by label name
			var config models.ChannelConfig
			result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)

			// If no config found for the specified label name, create a new one
			if result.Error != nil {
				log.Printf("config for channel(%s) and label(%s) not found: %v",
					channelID, labelName, result.Error)

				// New config creation is handled within each command processor
			}

			lang := getLang(&config)
			t := i18n.L(lang)

			switch subCommand {
			case "show":
				// Show current settings
				if isSubCommand && len(parts) == 1 {
					// If no label name is specified, show all labels
					showAllLabels(c, db, channelID, lang)
				} else {
					// Show settings for a specific label
					showConfig(c, db, channelID, labelName, lang)
				}

			case "set-mention":
				if params == "" {
					c.String(200, t("cmd.set_mention.usage", labelName))
					return
				}
				mentionID := strings.TrimSpace(params)
				setMention(c, db, channelID, labelName, mentionID, lang)

			case "add-reviewer":
				if params == "" {
					c.String(200, t("cmd.add_reviewer.usage", labelName))
					return
				}
				// Use regex to handle all space patterns
				re := regexp.MustCompile(`\s*,\s*`)
				reviewerIDs := re.ReplaceAllString(params, ",")

				// Also trim leading and trailing whitespace
				reviewerIDs = strings.TrimSpace(reviewerIDs)
				addReviewers(c, db, channelID, labelName, reviewerIDs, lang)

			case "show-reviewers":
				// Show the reviewer list
				showReviewers(c, db, channelID, labelName, lang)

			case "clear-reviewers":
				// Clear the reviewer list
				clearReviewers(c, db, channelID, labelName, lang)

			case "add-repo":
				if params == "" {
					c.String(200, t("cmd.add_repo.usage", labelName))
					return
				}
				repoName := params
				addRepository(c, db, channelID, labelName, repoName, lang)

			case "remove-repo":
				if params == "" {
					c.String(200, t("cmd.remove_repo.usage", labelName))
					return
				}
				repoName := params
				removeRepository(c, db, channelID, labelName, repoName, lang)

			case "set-label":
				// set-label is actually a rename operation for the label name
				if params == "" {
					c.String(200, t("cmd.set_label.usage", labelName))
					return
				}
				newLabelName := params
				changeLabelName(c, db, channelID, labelName, newLabelName, lang)

			case "activate":
				activateChannel(c, db, channelID, labelName, lang, true)

			case "deactivate":
				activateChannel(c, db, channelID, labelName, lang, false)

			case "set-reviewer-reminder-interval":
				if params == "" {
					c.String(200, t("cmd.set_reminder_interval.usage", labelName))
					return
				}
				setReminderInterval(c, db, channelID, labelName, strings.TrimSpace(params), lang, true)

			case "set-business-hours-start":
				if params == "" {
					c.String(200, t("cmd.set_business_hours_start.usage", labelName))
					return
				}
				setBusinessHoursStart(c, db, channelID, labelName, strings.TrimSpace(params), lang)

			case "set-business-hours-end":
				if params == "" {
					c.String(200, t("cmd.set_business_hours_end.usage", labelName))
					return
				}
				setBusinessHoursEnd(c, db, channelID, labelName, strings.TrimSpace(params), lang)

			case "set-timezone":
				if params == "" {
					c.String(200, t("cmd.set_timezone.usage", labelName))
					return
				}
				setTimezone(c, db, channelID, labelName, strings.TrimSpace(params), lang)

			case "map-user":
				mapUser(c, db, params, lang)

			case "show-user-mappings":
				showUserMappings(c, db, lang)

			case "remove-user-mapping":
				removeUserMapping(c, db, params, lang)

			case "set-required-approvals":
				if params == "" {
					c.String(200, t("cmd.set_required_approvals.usage", labelName))
					return
				}
				setRequiredApprovals(c, db, channelID, labelName, strings.TrimSpace(params), lang)

			case "set-language":
				if params == "" {
					c.String(200, t("cmd.set_language.usage", labelName))
					return
				}
				setLanguage(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-away":
				setAway(c, db, channelID, labelName, params, lang)

			case "unset-away":
				unsetAway(c, db, params, lang)

			case "show-availability":
				showAvailability(c, db, lang)

			default:
				c.String(200, t("cmd.unknown_with_help"))
			}

			return
		}

		// Try to get language from channel config for fallback message
	var fallbackConfig models.ChannelConfig
	fallbackLang := "ja"
	if err := db.Where("slack_channel_id = ?", channelID).First(&fallbackConfig).Error; err == nil {
		fallbackLang = getLang(&fallbackConfig)
	}
	c.String(200, i18n.TWithLang(fallbackLang, "cmd.unknown"))
	}
}

// parseCommand parses command text with quote support
func parseCommand(text string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(text); i++ {
		char := text[i]

		switch {
		case char == '"' || char == '\'':
			if !inQuote {
				// Start of quote
				inQuote = true
				quoteChar = char
			} else if char == quoteChar {
				// End of quote
				inQuote = false
				quoteChar = 0
			} else {
				// Treat different quote characters as regular characters
				current.WriteByte(char)
			}
		case char == ' ' && !inQuote:
			// Split on space (only when not inside quotes)
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(char)
		}
	}

	// Add the last part
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// getLang returns the language from the config, defaulting to "ja"
func getLang(config *models.ChannelConfig) string {
	if config != nil && config.Language != "" {
		return config.Language
	}
	return "ja"
}

// showHelp displays the help message
func showHelp(c *gin.Context, lang string) {
	t := i18n.L(lang)
	c.String(200, t("cmd.help"))
}

// showAllLabels displays all label configurations
func showAllLabels(c *gin.Context, db *gorm.DB, channelID, lang string) {
	t := i18n.L(lang)
	var configs []models.ChannelConfig

	err := db.Where("slack_channel_id = ?", channelID).Find(&configs).Error
	if err != nil {
		c.String(200, t("cmd.show.error"))
		return
	}

	if len(configs) == 0 {
		c.String(200, t("cmd.show.no_config"))
		return
	}

	response := t("cmd.show.header")

	for _, config := range configs {
		status := t("common.inactive")
		if config.IsActive {
			status = t("common.active")
		}

		response += fmt.Sprintf("• `%s` - %s (<@%s>)\n", config.LabelName, status, config.DefaultMentionID)
	}

	response += t("cmd.show.footer")

	c.String(200, response)
}

// showConfig displays the configuration
func showConfig(c *gin.Context, db *gorm.DB, channelID, labelName, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, t("cmd.show_config.no_config", labelName, labelName))
		return
	}

	status := t("common.inactive")
	if config.IsActive {
		status = t("common.active")
	}

	reviewerReminderInterval := config.ReviewerReminderInterval
	if reviewerReminderInterval <= 0 {
		reviewerReminderInterval = 30
	}

	timezone := config.Timezone
	if timezone == "" {
		timezone = "Asia/Tokyo" // Default timezone
	}

	requiredApprovals := config.RequiredApprovals
	if requiredApprovals <= 0 {
		requiredApprovals = 1
	}

	language := config.Language
	if language == "" {
		language = "ja"
	}

	response := t("cmd.show_config.response", labelName, status, config.DefaultMentionID, formatReviewerList(config.ReviewerList, lang),
		config.RepositoryList, reviewerReminderInterval, config.BusinessHoursStart, config.BusinessHoursEnd, timezone, requiredApprovals, language)

	c.String(200, response)
}

// formatReviewerList formats the reviewer list for display
func formatReviewerList(reviewerList, lang string) string {
	t := i18n.L(lang)
	if reviewerList == "" {
		return t("common.not_set")
	}

	reviewers := strings.Split(reviewerList, ",")
	formattedList := []string{}

	for _, reviewer := range reviewers {
		formattedList = append(formattedList, fmt.Sprintf("<@%s>", strings.TrimSpace(reviewer)))
	}

	return strings.Join(formattedList, ", ")
}

// cleanUserID converts a string to a clean user ID
func cleanUserID(userID string) string {
	// Trim whitespace
	userID = strings.TrimSpace(userID)

	// Handle team mention format <!subteam^ID|@name>
	if strings.HasPrefix(userID, "<!subteam^") && strings.Contains(userID, "|") && strings.HasSuffix(userID, ">") {
		parts := strings.Split(userID, "|")
		if len(parts) > 0 {
			id := strings.TrimPrefix(parts[0], "<!subteam^")
			// Return the team ID as-is
			return id
		}
	}

	// Handle regular user mention <@ID>
	if strings.HasPrefix(userID, "<@") && strings.HasSuffix(userID, ">") {
		return strings.TrimPrefix(strings.TrimSuffix(userID, ">"), "<@")
	}

	// Remove @ prefix if present
	userID = strings.TrimPrefix(userID, "@")

	// Remove commas if present
	userID = strings.ReplaceAll(userID, ",", "")

	return userID
}

// cleanupUserIDs cleans up multiple user IDs
func cleanupUserIDs(userIDs string) string {
	ids := strings.Split(userIDs, ",")
	cleanedIDs := make([]string, 0, len(ids))

	for _, id := range ids {
		cleaned := cleanUserID(strings.TrimSpace(id))
		if cleaned != "" {
			cleanedIDs = append(cleanedIDs, cleaned)
		}
	}

	return strings.Join(cleanedIDs, ",")
}

// addReviewers adds reviewers
func addReviewers(c *gin.Context, db *gorm.DB, channelID, labelName, reviewerIDs, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// Create new config if none exists yet
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			ReviewerList:   cleanupUserIDs(reviewerIDs), // Clean up user ID format
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.add_reviewer.created", labelName, formatReviewerList(config.ReviewerList, lang)))
		return
	}

	// Check the existing reviewer list
	currentReviewers := []string{}
	if config.ReviewerList != "" {
		currentReviewers = strings.Split(config.ReviewerList, ",")
		for i, r := range currentReviewers {
			currentReviewers[i] = strings.TrimSpace(r)
		}
	}

	// Add new reviewers
	newReviewers := strings.Split(reviewerIDs, ",")
	for _, newReviewer := range newReviewers {
		newReviewer = cleanUserID(strings.TrimSpace(newReviewer))
		alreadyExists := false

		for _, existingReviewer := range currentReviewers {
			if existingReviewer == newReviewer {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists && newReviewer != "" {
			currentReviewers = append(currentReviewers, newReviewer)
		}
	}

	// Save the updated list
	config.ReviewerList = strings.Join(currentReviewers, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.add_reviewer.updated", labelName, formatReviewerList(config.ReviewerList, lang)))
}

// showReviewers displays the reviewer list
func showReviewers(c *gin.Context, db *gorm.DB, channelID, labelName, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, t("cmd.show_reviewers.no_config", labelName, labelName))
		return
	}

	if config.ReviewerList == "" {
		c.String(200, t("cmd.show_reviewers.empty", labelName, labelName))
		return
	}

	response := t("cmd.show_reviewers.header", labelName, formatReviewerList(config.ReviewerList, lang))
	c.String(200, response)
}

// clearReviewers clears the reviewer list
func clearReviewers(c *gin.Context, db *gorm.DB, channelID, labelName, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, t("cmd.clear_reviewers.no_config", labelName))
		return
	}

	config.ReviewerList = ""
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.clear_reviewers.success", labelName))
}

// setMention sets the mention target
func setMention(c *gin.Context, db *gorm.DB, channelID, labelName, mentionID, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	// Clean up the mention ID
	cleanedMentionID := cleanUserID(mentionID)

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// Create new config
		config = models.ChannelConfig{
			ID:               uuid.NewString(),
			SlackChannelID:   channelID,
			LabelName:        labelName,
			DefaultMentionID: cleanedMentionID,
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		db.Create(&config)

		// Determine display format based on whether it's a team mention
		var mentionDisplay string
		if strings.HasPrefix(mentionID, "<!subteam^") {
			mentionDisplay = fmt.Sprintf("<!subteam^%s>", cleanedMentionID)
		} else {
			mentionDisplay = fmt.Sprintf("<@%s>", cleanedMentionID)
		}

		c.String(200, t("cmd.set_mention.created", labelName, mentionDisplay))
		return
	}

	// Update existing config
	config.DefaultMentionID = cleanedMentionID
	config.UpdatedAt = time.Now()
	db.Save(&config)

	// Determine display format based on whether it's a team mention
	var mentionDisplay string
	if strings.HasPrefix(mentionID, "<!subteam^") {
		mentionDisplay = fmt.Sprintf("<!subteam^%s>", cleanedMentionID)
	} else {
		mentionDisplay = fmt.Sprintf("<@%s>", cleanedMentionID)
	}

	c.String(200, t("cmd.set_mention.updated", labelName, mentionDisplay))
}

// addRepository adds a repository
func addRepository(c *gin.Context, db *gorm.DB, channelID, labelName, repoNames, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// Create new config if none exists yet
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			RepositoryList: repoNames,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.add_repo.created", labelName, repoNames))
		return
	}

	// Use regex to handle all space patterns
	re := regexp.MustCompile(`\s*,\s*`)
	repoNames = re.ReplaceAllString(repoNames, ",")

	// Also trim leading and trailing whitespace
	repoNames = strings.TrimSpace(repoNames)

	// Check the existing repository list
	currentRepos := []string{}
	if config.RepositoryList != "" {
		currentRepos = strings.Split(config.RepositoryList, ",")
		for i, r := range currentRepos {
			currentRepos[i] = strings.TrimSpace(r)
		}
	}

	// Add new repositories
	newRepos := strings.Split(repoNames, ",")
	addedRepos := []string{}
	alreadyExistsRepos := []string{}

	for _, newRepo := range newRepos {
		newRepo = strings.TrimSpace(newRepo)
		if newRepo == "" {
			continue
		}

		alreadyExists := false
		for _, existingRepo := range currentRepos {
			if existingRepo == newRepo {
				alreadyExists = true
				alreadyExistsRepos = append(alreadyExistsRepos, newRepo)
				break
			}
		}

		if !alreadyExists {
			currentRepos = append(currentRepos, newRepo)
			addedRepos = append(addedRepos, newRepo)
		}
	}

	// Save the updated list
	config.RepositoryList = strings.Join(currentRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	// Build the response message
	var response string
	if len(addedRepos) > 0 {
		response = t("cmd.add_repo.added", labelName, strings.Join(addedRepos, "`, `"))
	}

	if len(alreadyExistsRepos) > 0 {
		if response != "" {
			response += "\n\n"
		}
		response += t("cmd.add_repo.already_exists", strings.Join(alreadyExistsRepos, "`, `"))
	}

	if response == "" {
		response = t("cmd.add_repo.no_valid")
	}

	c.String(200, response)
}

// removeRepository removes a repository
func removeRepository(c *gin.Context, db *gorm.DB, channelID, labelName, repoName, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		c.String(200, t("cmd.remove_repo.no_config", labelName))
		return
	}

	if config.RepositoryList == "" {
		c.String(200, t("cmd.remove_repo.empty", labelName))
		return
	}

	// Parse the repository list
	repos := strings.Split(config.RepositoryList, ",")
	newRepos := []string{}
	found := false

	for _, r := range repos {
		if strings.TrimSpace(r) != repoName {
			newRepos = append(newRepos, strings.TrimSpace(r))
		} else {
			found = true
		}
	}

	if !found {
		c.String(200, t("cmd.remove_repo.not_found", repoName, labelName))
		return
	}

	// Save the new list
	config.RepositoryList = strings.Join(newRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.remove_repo.success", labelName, repoName))
}

// changeLabelName renames a label
func changeLabelName(c *gin.Context, db *gorm.DB, channelID, oldLabelName, newLabelName, lang string) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	// Get the current config
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, oldLabelName).First(&config)
	if result.Error != nil {
		c.String(200, t("cmd.set_label.no_config", oldLabelName))
		return
	}

	// Check if a config already exists with the new label name
	var existingConfig models.ChannelConfig
	existingResult := db.Where("slack_channel_id = ? AND label_name = ?", channelID, newLabelName).First(&existingConfig)
	if existingResult.Error == nil {
		c.String(200, t("cmd.set_label.already_exists", newLabelName))
		return
	}

	// Update the label name
	config.LabelName = newLabelName
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.set_label.success", oldLabelName, newLabelName))
}

// activateChannel toggles channel activation on/off
func activateChannel(c *gin.Context, db *gorm.DB, channelID, labelName, lang string, active bool) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		if active {
			c.String(200, t("cmd.activate.no_config", labelName, labelName))
		} else {
			c.String(200, t("cmd.deactivate.no_config", labelName))
		}
		return
	}

	// Update existing config
	config.IsActive = active
	config.UpdatedAt = time.Now()
	db.Save(&config)

	if active {
		c.String(200, t("cmd.activate.success", labelName))
	} else {
		c.String(200, t("cmd.deactivate.success", labelName))
	}
}

// setReminderInterval sets the reminder frequency
func setReminderInterval(c *gin.Context, db *gorm.DB, channelID, labelName, intervalStr, lang string, isReviewer bool) {
	t := i18n.L(lang)
	var config models.ChannelConfig

	// Convert to number
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		c.String(200, t("cmd.set_reminder_interval.invalid"))
		return
	}

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// Create new config if none exists yet
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if isReviewer {
			config.ReviewerReminderInterval = interval
		} else {
			config.ReminderInterval = interval
		}

		db.Create(&config)

		if isReviewer {
			c.String(200, t("cmd.set_reminder_interval.reviewer_set", interval))
		} else {
			c.String(200, t("cmd.set_reminder_interval.pending_set", interval))
		}
		return
	}

	// Update existing config
	if isReviewer {
		config.ReviewerReminderInterval = interval
		c.String(200, t("cmd.set_reminder_interval.reviewer_set", interval))
	} else {
		config.ReminderInterval = interval
		c.String(200, t("cmd.set_reminder_interval.pending_set", interval))
	}

	config.UpdatedAt = time.Now()
	db.Save(&config)
}

// setBusinessHoursStart sets the business hours start time
func setBusinessHoursStart(c *gin.Context, db *gorm.DB, channelID, labelName, startTime, lang string) {
	t := i18n.L(lang)
	if !isValidTimeFormat(startTime) {
		c.String(200, t("cmd.time_format_invalid", "09:00"))
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)

	if result.Error != nil {
		// Create new config
		config = models.ChannelConfig{
			ID:                 uuid.NewString(),
			SlackChannelID:     channelID,
			LabelName:          labelName,
			BusinessHoursStart: startTime,
			BusinessHoursEnd:   "18:00",
			Timezone:           "Asia/Tokyo",
			IsActive:           true,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.set_business_hours_start.set", labelName, startTime))
		return
	}

	config.BusinessHoursStart = startTime
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.set_business_hours_start.updated", labelName, startTime))
}

// setBusinessHoursEnd sets the business hours end time
func setBusinessHoursEnd(c *gin.Context, db *gorm.DB, channelID, labelName, endTime, lang string) {
	t := i18n.L(lang)
	if !isValidTimeFormat(endTime) {
		c.String(200, t("cmd.time_format_invalid", "18:00"))
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)

	if result.Error != nil {
		config = models.ChannelConfig{
			ID:                 uuid.NewString(),
			SlackChannelID:     channelID,
			LabelName:          labelName,
			BusinessHoursStart: "09:00",
			BusinessHoursEnd:   endTime,
			Timezone:           "Asia/Tokyo",
			IsActive:           true,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.set_business_hours_end.set", labelName, endTime))
		return
	}

	// Update existing config
	config.BusinessHoursEnd = endTime
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.set_business_hours_end.updated", labelName, endTime))
}

// setTimezone sets the timezone
func setTimezone(c *gin.Context, db *gorm.DB, channelID, labelName, timezone, lang string) {
	t := i18n.L(lang)
	// Validate the timezone
	if !isValidTimezone(timezone) {
		c.String(200, t("cmd.set_timezone.invalid"))
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)

	if result.Error != nil {
		// Create new config
		config = models.ChannelConfig{
			ID:                 uuid.NewString(),
			SlackChannelID:     channelID,
			LabelName:          labelName,
			Timezone:           timezone,
			BusinessHoursStart: "09:00",
			BusinessHoursEnd:   "18:00",
			IsActive:           true,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.set_timezone.set", labelName, timezone))
		return
	}

	// Update existing config
	config.Timezone = timezone
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.set_timezone.updated", labelName, timezone))
}

// isValidTimezone validates the timezone
func isValidTimezone(timezone string) bool {
	_, err := time.LoadLocation(timezone)
	return err == nil
}

// isValidTimeFormat validates the time format (HH:MM)
func isValidTimeFormat(timeStr string) bool {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return false
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return false
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return false
	}

	return true
}

func mapUser(c *gin.Context, db *gorm.DB, params, lang string) {
	t := i18n.L(lang)
	parts := strings.Fields(params)
	if len(parts) < 2 {
		c.String(200, t("cmd.map_user.usage"))
		return
	}

	githubUsername := strings.TrimSpace(parts[0])
	slackUserID := cleanUserID(parts[1])

	if githubUsername == "" || slackUserID == "" {
		c.String(200, t("cmd.map_user.invalid"))
		return
	}

	var existingMapping models.UserMapping
	result := db.Where("github_username = ?", githubUsername).First(&existingMapping)

	if result.Error == nil {
		existingMapping.SlackUserID = slackUserID
		existingMapping.UpdatedAt = time.Now()
		db.Save(&existingMapping)
		c.String(200, t("cmd.map_user.updated", githubUsername, slackUserID))
		return
	}

	newMapping := models.UserMapping{
		ID:             uuid.NewString(),
		GithubUsername: githubUsername,
		SlackUserID:    slackUserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := db.Create(&newMapping).Error; err != nil {
		log.Printf("failed to create user mapping: %v", err)
		c.String(200, t("cmd.map_user.create_error"))
		return
	}

	c.String(200, t("cmd.map_user.created", githubUsername, slackUserID))
}

func showUserMappings(c *gin.Context, db *gorm.DB, lang string) {
	t := i18n.L(lang)
	var mappings []models.UserMapping

	if err := db.Find(&mappings).Error; err != nil {
		log.Printf("failed to get user mappings: %v", err)
		c.String(200, t("cmd.show_user_mappings.error"))
		return
	}

	if len(mappings) == 0 {
		c.String(200, t("cmd.show_user_mappings.empty"))
		return
	}

	response := t("cmd.show_user_mappings.header")
	for _, mapping := range mappings {
		response += fmt.Sprintf("• GitHub: `%s` → Slack: <@%s>\n", mapping.GithubUsername, mapping.SlackUserID)
	}

	c.String(200, response)
}

func removeUserMapping(c *gin.Context, db *gorm.DB, githubUsername, lang string) {
	t := i18n.L(lang)
	githubUsername = strings.TrimSpace(githubUsername)

	if githubUsername == "" {
		c.String(200, t("cmd.remove_user_mapping.usage"))
		return
	}

	var mapping models.UserMapping
	result := db.Where("github_username = ?", githubUsername).First(&mapping)

	if result.Error != nil {
		c.String(200, t("cmd.remove_user_mapping.not_found", githubUsername))
		return
	}

	if err := db.Delete(&mapping).Error; err != nil {
		log.Printf("failed to delete user mapping: %v", err)
		c.String(200, t("cmd.remove_user_mapping.error"))
		return
	}

	c.String(200, t("cmd.remove_user_mapping.success", githubUsername))
}

// setAway marks a user as away/on leave
func setAway(c *gin.Context, db *gorm.DB, channelID, labelName, params, lang string) {
	t := i18n.L(lang)
	if params == "" {
		c.String(200, t("cmd.set_away.usage"))
		return
	}

	parts := strings.Fields(params)
	if len(parts) == 0 {
		c.String(200, t("cmd.set_away.no_user"))
		return
	}

	slackUserID := cleanUserID(parts[0])
	if slackUserID == "" {
		c.String(200, t("common.invalid_user_id"))
		return
	}

	var awayFrom *time.Time
	var awayUntil *time.Time
	var reason string

	// Get timezone from channel config
	timezone := "Asia/Tokyo"
	var tzConfig models.ChannelConfig
	if err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&tzConfig).Error; err == nil {
		if tzConfig.Timezone != "" {
			timezone = tzConfig.Timezone
		}
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("invalid timezone %q, falling back to UTC: %v", timezone, err)
		loc = time.UTC
	}

	nowLocal := time.Now().In(loc)
	todayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	hasOn := false

	// Parse parameters: "from", "until", "on", and "reason" keywords
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "from":
			if hasOn {
				c.String(200, t("cmd.set_away.conflicting_keywords"))
				return
			}
			if i+1 >= len(parts) {
				c.String(200, t("cmd.set_away.missing_date"))
				return
			}
			i++
			parsed, err := time.Parse("2006-01-02", parts[i])
			if err != nil {
				c.String(200, t("cmd.set_away.invalid_date"))
				return
			}
			startOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
			if startOfDay.Before(todayStart) {
				c.String(200, t("cmd.set_away.past_date"))
				return
			}
			awayFrom = &startOfDay
		case "until":
			if hasOn {
				c.String(200, t("cmd.set_away.conflicting_keywords"))
				return
			}
			if i+1 >= len(parts) {
				c.String(200, t("cmd.set_away.missing_date"))
				return
			}
			i++
			parsed, err := time.Parse("2006-01-02", parts[i])
			if err != nil {
				c.String(200, t("cmd.set_away.invalid_date"))
				return
			}
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc)
			if endOfDay.Before(nowLocal) {
				c.String(200, t("cmd.set_away.past_date"))
				return
			}
			awayUntil = &endOfDay
		case "on":
			if awayFrom != nil || awayUntil != nil {
				c.String(200, t("cmd.set_away.conflicting_keywords"))
				return
			}
			if i+1 >= len(parts) {
				c.String(200, t("cmd.set_away.missing_date"))
				return
			}
			i++
			parsed, err := time.Parse("2006-01-02", parts[i])
			if err != nil {
				c.String(200, t("cmd.set_away.invalid_date"))
				return
			}
			startOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc)
			if endOfDay.Before(nowLocal) {
				c.String(200, t("cmd.set_away.past_date"))
				return
			}
			awayFrom = &startOfDay
			awayUntil = &endOfDay
			hasOn = true
		case "reason":
			if i+1 < len(parts) {
				reason = strings.Join(parts[i+1:], " ")
				i = len(parts) // End loop
			}
		}
	}

	// Validate from < until when both are specified
	if awayFrom != nil && awayUntil != nil && !awayFrom.Before(*awayUntil) {
		c.String(200, t("cmd.set_away.from_after_until"))
		return
	}

	// Update if existing record exists, otherwise create new (upsert)
	var existing models.ReviewerAvailability
	result := db.Where("slack_user_id = ?", slackUserID).First(&existing)
	if result.Error == nil {
		existing.AwayFrom = awayFrom
		existing.AwayUntil = awayUntil
		existing.Reason = reason
		existing.UpdatedAt = time.Now()
		if err := db.Save(&existing).Error; err != nil {
			log.Printf("failed to update reviewer availability: slackUserID=%s channelID=%s err=%v", slackUserID, channelID, err)
			c.String(200, t("cmd.set_away.update_error"))
			return
		}
	} else {
		record := models.ReviewerAvailability{
			ID:          uuid.NewString(),
			SlackUserID: slackUserID,
			AwayFrom:    awayFrom,
			AwayUntil:   awayUntil,
			Reason:      reason,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := db.Create(&record).Error; err != nil {
			log.Printf("failed to create reviewer availability: slackUserID=%s channelID=%s err=%v", slackUserID, channelID, err)
			c.String(200, t("cmd.set_away.create_error"))
			return
		}
	}

	// Build response message
	openParen, closeParen := "（", "）"
	if lang == "en" {
		openParen, closeParen = " (", ")"
	}
	response := t("cmd.set_away.success", slackUserID)
	response += openParen + formatDateRange(awayFrom, awayUntil, t)

	if reason != "" {
		response += t("common.reason", reason) + closeParen
	} else {
		response += closeParen
	}

	c.String(200, response)
}

// unsetAway removes a user's away/leave status
func unsetAway(c *gin.Context, db *gorm.DB, params, lang string) {
	t := i18n.L(lang)
	if params == "" {
		c.String(200, t("cmd.unset_away.usage"))
		return
	}

	slackUserID := cleanUserID(strings.TrimSpace(params))
	if slackUserID == "" {
		c.String(200, t("common.invalid_user_id"))
		return
	}

	result := db.Unscoped().Where("slack_user_id = ?", slackUserID).Delete(&models.ReviewerAvailability{})
	if result.RowsAffected == 0 {
		c.String(200, t("cmd.unset_away.not_set", slackUserID))
		return
	}

	c.String(200, t("cmd.unset_away.success", slackUserID))
}

// formatDateRange returns a human-readable date range string for away periods.
func formatDateRange(awayFrom, awayUntil *time.Time, t func(string, ...interface{}) string) string {
	isSameDay := awayFrom != nil && awayUntil != nil &&
		awayFrom.Year() == awayUntil.Year() && awayFrom.YearDay() == awayUntil.YearDay()

	switch {
	case isSameDay:
		return t("common.on_date", awayFrom.Format("2006-01-02"))
	case awayFrom != nil && awayUntil != nil:
		return t("common.from_until", awayFrom.Format("2006-01-02"), awayUntil.Format("2006-01-02"))
	case awayFrom != nil:
		return t("common.from_until", awayFrom.Format("2006-01-02"), t("common.indefinite"))
	case awayUntil != nil:
		return t("common.until", awayUntil.Format("2006-01-02"))
	default:
		return t("common.indefinite")
	}
}

// showAvailability displays a list of users currently on leave
func showAvailability(c *gin.Context, db *gorm.DB, lang string) {
	t := i18n.L(lang)
	var records []models.ReviewerAvailability
	now := time.Now()

	// Get non-expired records (currently away or scheduled for the future)
	db.Where("away_until IS NULL OR away_until > ?", now).Find(&records)

	if len(records) == 0 {
		c.String(200, t("cmd.show_availability.empty"))
		return
	}

	response := t("cmd.show_availability.header")
	for _, r := range records {
		// Determine status: scheduled (AwayFrom is in the future) or currently away
		isScheduled := r.AwayFrom != nil && r.AwayFrom.After(now)
		var statusLabel string
		if isScheduled {
			statusLabel = t("cmd.show_availability.status_scheduled")
		} else {
			statusLabel = t("cmd.show_availability.status_away")
		}

		line := fmt.Sprintf("• <@%s> [%s] ", r.SlackUserID, statusLabel)
		line += formatDateRange(r.AwayFrom, r.AwayUntil, t)

		if r.Reason != "" {
			line += t("common.reason_paren", r.Reason)
		}
		response += line + "\n"
	}

	c.String(200, response)
}

// setRequiredApprovals sets the number of required approvals
func setRequiredApprovals(c *gin.Context, db *gorm.DB, channelID, labelName, countStr, lang string) {
	t := i18n.L(lang)
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 || count > 10 {
		c.String(200, t("cmd.set_required_approvals.invalid"))
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		config = models.ChannelConfig{
			ID:                uuid.NewString(),
			SlackChannelID:    channelID,
			LabelName:         labelName,
			RequiredApprovals: count,
			IsActive:          true,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
		db.Create(&config)
		c.String(200, t("cmd.set_required_approvals.set", labelName, count))
		return
	}

	config.RequiredApprovals = count
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, t("cmd.set_required_approvals.updated", labelName, count))
}

// setLanguage sets the language for the channel config
func setLanguage(c *gin.Context, db *gorm.DB, channelID, labelName, newLang string) {
	if newLang != "ja" && newLang != "en" {
		// Use current config language for the error message
		var currentConfig models.ChannelConfig
		currentLang := "ja"
		if err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&currentConfig).Error; err == nil {
			currentLang = getLang(&currentConfig)
		}
		t := i18n.L(currentLang)
		c.String(200, t("cmd.set_language.invalid"))
		return
	}
	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			Language:        newLang,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if err := db.Create(&config).Error; err != nil {
			log.Printf("failed to create config for set-language: %v", err)
			c.String(200, "Error: failed to save language setting")
			return
		}
		t := i18n.L(newLang)
		c.String(200, t("cmd.set_language.set", labelName, newLang))
		return
	}

	config.Language = newLang
	config.UpdatedAt = time.Now()
	if err := db.Save(&config).Error; err != nil {
		log.Printf("failed to update config for set-language: %v", err)
		c.String(200, "Error: failed to save language setting")
		return
	}

	t := i18n.L(newLang)
	c.String(200, t("cmd.set_language.updated", labelName, newLang))
}
