package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
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
				showHelp(c)
				return
			}

			// Determine whether the first argument is a subcommand or a label name
			potentialSubCommands := []string{"show", "help", "set-mention", "add-reviewer",
				"show-reviewers", "clear-reviewers", "add-repo", "remove-repo",
				"set-label", "activate", "deactivate", "set-reviewer-reminder-interval",
				"set-business-hours-start", "set-business-hours-end", "set-timezone",
				"map-user", "show-user-mappings", "remove-user-mapping",
				"set-required-approvals",
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
				// Show help
				showHelp(c)
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

			switch subCommand {
			case "show":
				// Show current settings
				if isSubCommand && len(parts) == 1 {
					// If no label name is specified, show all labels
					showAllLabels(c, db, channelID)
				} else {
					// Show settings for a specific label
					showConfig(c, db, channelID, labelName)
				}

			case "set-mention":
				if params == "" {
					c.String(200, "メンション先のユーザーIDを指定してください。例: /slack-review-notify "+labelName+" set-mention @user")
					return
				}
				mentionID := strings.TrimSpace(params)
				setMention(c, db, channelID, labelName, mentionID)

			case "add-reviewer":
				if params == "" {
					c.String(200, "レビュワーのユーザーIDをカンマ区切りで指定してください。例: /slack-review-notify "+labelName+" add-reviewer @user1,@user2")
					return
				}
				// Use regex to handle all space patterns
				re := regexp.MustCompile(`\s*,\s*`)
				reviewerIDs := re.ReplaceAllString(params, ",")

				// Also trim leading and trailing whitespace
				reviewerIDs = strings.TrimSpace(reviewerIDs)
				addReviewers(c, db, channelID, labelName, reviewerIDs)

			case "show-reviewers":
				// Show the reviewer list
				showReviewers(c, db, channelID, labelName)

			case "clear-reviewers":
				// Clear the reviewer list
				clearReviewers(c, db, channelID, labelName)

			case "add-repo":
				if params == "" {
					c.String(200, "リポジトリ名をカンマ区切りで指定してください。例: /slack-review-notify "+labelName+" add-repo owner/repo1,owner/repo2")
					return
				}
				repoName := params
				addRepository(c, db, channelID, labelName, repoName)

			case "remove-repo":
				if params == "" {
					c.String(200, "リポジトリ名を指定してください。例: /slack-review-notify "+labelName+" remove-repo owner/repo")
					return
				}
				repoName := params
				removeRepository(c, db, channelID, labelName, repoName)

			case "set-label":
				// set-label is actually a rename operation for the label name
				if params == "" {
					c.String(200, "新しいラベル名を指定してください。例: /slack-review-notify "+labelName+" set-label new-label-name")
					return
				}
				newLabelName := params
				changeLabelName(c, db, channelID, labelName, newLabelName)

			case "activate":
				activateChannel(c, db, channelID, labelName, true)

			case "deactivate":
				activateChannel(c, db, channelID, labelName, false)

			case "set-reviewer-reminder-interval":
				if params == "" {
					c.String(200, "レビュワー割り当て後のリマインド頻度を分単位で指定してください。例: /slack-review-notify "+labelName+" set-reviewer-reminder-interval 30")
					return
				}
				setReminderInterval(c, db, channelID, labelName, strings.TrimSpace(params), true)

			case "set-business-hours-start":
				if params == "" {
					c.String(200, "営業開始時間を指定してください。例: /slack-review-notify "+labelName+" set-business-hours-start 09:00")
					return
				}
				setBusinessHoursStart(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-business-hours-end":
				if params == "" {
					c.String(200, "営業終了時間を指定してください。例: /slack-review-notify "+labelName+" set-business-hours-end 18:00")
					return
				}
				setBusinessHoursEnd(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-timezone":
				if params == "" {
					c.String(200, "タイムゾーンを指定してください。例: /slack-review-notify "+labelName+" set-timezone Asia/Tokyo")
					return
				}
				setTimezone(c, db, channelID, labelName, strings.TrimSpace(params))

			case "map-user":
				mapUser(c, db, params)

			case "show-user-mappings":
				showUserMappings(c, db)

			case "remove-user-mapping":
				removeUserMapping(c, db, params)

			case "set-required-approvals":
				if params == "" {
					c.String(200, "必要なapprove数を指定してください。例: /slack-review-notify "+labelName+" set-required-approvals 2")
					return
				}
				setRequiredApprovals(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-away":
				setAway(c, db, channelID, labelName, params)

			case "unset-away":
				unsetAway(c, db, params)

			case "show-availability":
				showAvailability(c, db)

			default:
				c.String(200, "不明なコマンドです。/slack-review-notify help で使い方を確認してください。")
			}

			return
		}

		c.String(200, "不明なコマンドです。")
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

// showHelp displays the help message
func showHelp(c *gin.Context) {
	help := `*Review通知Bot設定コマンド*
コマンド形式: /slack-review-notify [ラベル名] サブコマンド [引数]

*スペースを含むラベル名について*
スペースを含むラベル名を使用する場合は、クォート（"または'）で囲んでください:
- 例: /slack-review-notify "needs review" set-mention @team
- 例: /slack-review-notify 'security review' add-reviewer @security

*初期設定（必須）*
通知を受けるには最低限以下の設定が必要です：
:point_right: *1. メンション先の設定*
   /slack-review-notify [ラベル名] set-mention @user

:point_right: *2. 対象リポジトリの追加（重要！）*
   /slack-review-notify [ラベル名] add-repo owner/repo

:information_source: この2つを設定しないと通知は送信されません

*複数ラベル設定の使い方*
このチャンネル内で複数の異なるラベルごとに独立した設定を持つことができます:
- 例1: /slack-review-notify bug set-mention @バグチーム → bugラベル用の設定
- 例2: /slack-review-notify feature set-mention @開発チーム → featureラベル用の設定
- 例3: /slack-review-notify security-review set-mention @セキュリティチーム → security-reviewラベル用の設定

*複数ラベルAND条件の設定*
カンマ区切りで複数のラベルを指定することで、全てのラベルが付いている場合のみ通知します:
- 例: /slack-review-notify "hoge-project,needs-review" set-mention @team
  → PRに「hoge-project」と「needs-review」の両方のラベルがある場合のみ通知
- 例: /slack-review-notify "frontend,urgent,needs-review" add-repo owner/app
  → 3つのラベル全てが必要

*全コマンド一覧*
*基本操作:*
• /slack-review-notify show - このチャンネルの全ラベル設定を表示
• /slack-review-notify [ラベル名] show - 指定ラベルの詳細設定を表示

*必須設定:*
• /slack-review-notify [ラベル名] set-mention @user - メンション先を設定
• /slack-review-notify [ラベル名] add-repo owner/repo1,owner/repo2 - 対象リポジトリを追加

*レビュワー管理:*
• /slack-review-notify [ラベル名] add-reviewer @user1,@user2 - レビュワーを追加
• /slack-review-notify [ラベル名] show-reviewers - レビュワー一覧を表示
• /slack-review-notify [ラベル名] clear-reviewers - レビュワーをクリア

*高度な設定:*
• /slack-review-notify [ラベル名] remove-repo owner/repo - リポジトリを削除
• /slack-review-notify [ラベル名] set-label 新ラベル名 - ラベル名を変更
• /slack-review-notify [ラベル名] set-reviewer-reminder-interval 30 - リマインド頻度設定（分）
• /slack-review-notify [ラベル名] set-business-hours-start 09:00 - 営業開始時間を設定
• /slack-review-notify [ラベル名] set-business-hours-end 18:00 - 営業終了時間を設定
• /slack-review-notify [ラベル名] set-timezone Asia/Tokyo - タイムゾーンを設定
• /slack-review-notify [ラベル名] set-required-approvals N - 必要なapprove数を設定（1〜10）
• /slack-review-notify [ラベル名] activate - 通知を有効化
• /slack-review-notify [ラベル名] deactivate - 通知を無効化

*ユーザーマッピング（PR作成者の通知用）:*
• /slack-review-notify map-user <github-username> @slack-user - GitHubユーザーとSlackユーザーを紐付け
• /slack-review-notify show-user-mappings - 登録済みのユーザーマッピング一覧を表示
• /slack-review-notify remove-user-mapping <github-username> - ユーザーマッピングを削除

*休暇管理:*
• /slack-review-notify set-away @user [until YYYY-MM-DD] [reason 理由] - ユーザーを休暇に設定
• /slack-review-notify unset-away @user - ユーザーの休暇を解除
• /slack-review-notify show-availability - 休暇中のユーザー一覧を表示

[ラベル名]を省略すると「needs-review」というデフォルトのラベルを使用します`

	c.String(200, help)
}

// showAllLabels displays all label configurations
func showAllLabels(c *gin.Context, db *gorm.DB, channelID string) {
	var configs []models.ChannelConfig

	err := db.Where("slack_channel_id = ?", channelID).Find(&configs).Error
	if err != nil {
		c.String(200, "設定の取得中にエラーが発生しました。")
		return
	}

	if len(configs) == 0 {
		c.String(200, "このチャンネルにはまだ設定がありません。/slack-review-notify [ラベル名] set-mention コマンドで設定を開始してください。")
		return
	}

	response := "*このチャンネルで設定済みのラベル*\n"

	for _, config := range configs {
		status := "無効"
		if config.IsActive {
			status = "有効"
		}

		response += fmt.Sprintf("• `%s` - %s (<@%s>)\n", config.LabelName, status, config.DefaultMentionID)
	}

	response += "\n特定のラベルの詳細設定を確認するには: `/slack-review-notify [ラベル名] show`"

	c.String(200, response)
}

// showConfig displays the configuration
func showConfig(c *gin.Context, db *gorm.DB, channelID, labelName string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s set-mention コマンドで設定を開始してください。", labelName, labelName))
		return
	}

	status := "無効"
	if config.IsActive {
		status = "有効"
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

	response := fmt.Sprintf(`*このチャンネルのラベル「%s」のレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- レビュワーリスト: %s
- 通知対象リポジトリ: %s
- レビュワー割り当て後のリマインド頻度: %d分
- 営業時間: %s - %s (%s)
- 必要なapprove数: %d`,
		labelName, status, config.DefaultMentionID, formatReviewerList(config.ReviewerList),
		config.RepositoryList, reviewerReminderInterval, config.BusinessHoursStart, config.BusinessHoursEnd, timezone, requiredApprovals)

	c.String(200, response)
}

// formatReviewerList formats the reviewer list for display
func formatReviewerList(reviewerList string) string {
	if reviewerList == "" {
		return "未設定"
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
func addReviewers(c *gin.Context, db *gorm.DB, channelID, labelName, reviewerIDs string) {
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
		c.String(200, fmt.Sprintf("ラベル「%s」のレビュワーリストを設定しました: %s", labelName, formatReviewerList(config.ReviewerList)))
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

	c.String(200, fmt.Sprintf("ラベル「%s」のレビュワーリストを更新しました: %s", labelName, formatReviewerList(config.ReviewerList)))
}

// showReviewers displays the reviewer list
func showReviewers(c *gin.Context, db *gorm.DB, channelID, labelName string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s add-reviewer コマンドで設定を開始してください。", labelName, labelName))
		return
	}

	if config.ReviewerList == "" {
		c.String(200, fmt.Sprintf("現在レビュワーはラベル「%s」に登録されていません。/slack-review-notify %s add-reviewer コマンドで追加してください。", labelName, labelName))
		return
	}

	response := fmt.Sprintf("*このチャンネルのラベル「%s」のレビュワーリスト*\n%s", labelName, formatReviewerList(config.ReviewerList))
	c.String(200, response)
}

// clearReviewers clears the reviewer list
func clearReviewers(c *gin.Context, db *gorm.DB, channelID, labelName string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config).Error
	if err != nil {
		c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。", labelName))
		return
	}

	config.ReviewerList = ""
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」のレビュワーリストをクリアしました。", labelName))
}

// setMention sets the mention target
func setMention(c *gin.Context, db *gorm.DB, channelID, labelName, mentionID string) {
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

		c.String(200, fmt.Sprintf("ラベル「%s」のメンション先を %s に設定しました。", labelName, mentionDisplay))
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

	c.String(200, fmt.Sprintf("ラベル「%s」のメンション先を %s に更新しました。", labelName, mentionDisplay))
}

// addRepository adds a repository
func addRepository(c *gin.Context, db *gorm.DB, channelID, labelName, repoNames string) {
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
		c.String(200, fmt.Sprintf("ラベル「%s」の通知対象リポジトリに `%s` を追加しました。", labelName, repoNames))
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
		response = fmt.Sprintf("ラベル「%s」の通知対象リポジトリに以下を追加しました:\n`%s`", labelName, strings.Join(addedRepos, "`, `"))
	}

	if len(alreadyExistsRepos) > 0 {
		if response != "" {
			response += "\n\n"
		}
		response += fmt.Sprintf("以下のリポジトリは既に通知対象でした:\n`%s`", strings.Join(alreadyExistsRepos, "`, `"))
	}

	if response == "" {
		response = "有効なリポジトリ名が指定されませんでした。"
	}

	c.String(200, response)
}

// removeRepository removes a repository
func removeRepository(c *gin.Context, db *gorm.DB, channelID, labelName, repoName string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。", labelName))
		return
	}

	if config.RepositoryList == "" {
		c.String(200, fmt.Sprintf("ラベル「%s」の通知対象リポジトリは設定されていません。", labelName))
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
		c.String(200, fmt.Sprintf("リポジトリ `%s` はラベル「%s」の通知対象ではありません。", repoName, labelName))
		return
	}

	// Save the new list
	config.RepositoryList = strings.Join(newRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の通知対象リポジトリから `%s` を削除しました。", labelName, repoName))
}

// changeLabelName renames a label
func changeLabelName(c *gin.Context, db *gorm.DB, channelID, oldLabelName, newLabelName string) {
	var config models.ChannelConfig

	// Get the current config
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, oldLabelName).First(&config)
	if result.Error != nil {
		c.String(200, fmt.Sprintf("ラベル「%s」の設定はこのチャンネルに存在しません。", oldLabelName))
		return
	}

	// Check if a config already exists with the new label name
	var existingConfig models.ChannelConfig
	existingResult := db.Where("slack_channel_id = ? AND label_name = ?", channelID, newLabelName).First(&existingConfig)
	if existingResult.Error == nil {
		c.String(200, fmt.Sprintf("ラベル「%s」の設定は既に存在します。別のラベル名を指定してください。", newLabelName))
		return
	}

	// Update the label name
	config.LabelName = newLabelName
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル名を「%s」から「%s」に変更しました。", oldLabelName, newLabelName))
}

// activateChannel toggles channel activation on/off
func activateChannel(c *gin.Context, db *gorm.DB, channelID, labelName string, active bool) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		if active {
			c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s set-mention コマンドで設定を開始してください。", labelName, labelName))
		} else {
			c.String(200, fmt.Sprintf("このチャンネルのラベル「%s」の設定はまだありません。", labelName))
		}
		return
	}

	// Update existing config
	config.IsActive = active
	config.UpdatedAt = time.Now()
	db.Save(&config)

	if active {
		c.String(200, fmt.Sprintf("このチャンネルでのラベル「%s」のレビュー通知を有効化しました。", labelName))
	} else {
		c.String(200, fmt.Sprintf("このチャンネルでのラベル「%s」のレビュー通知を無効化しました。", labelName))
	}
}

// setReminderInterval sets the reminder frequency
func setReminderInterval(c *gin.Context, db *gorm.DB, channelID, labelName, intervalStr string, isReviewer bool) {
	var config models.ChannelConfig

	// Convert to number
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		c.String(200, "リマインド頻度は1以上の整数で指定してください。")
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
			c.String(200, fmt.Sprintf("レビュワー割り当て後のリマインド頻度を %d分 に設定しました。", interval))
		} else {
			c.String(200, fmt.Sprintf("レビュワー募集中のリマインド頻度を %d分 に設定しました。", interval))
		}
		return
	}

	// Update existing config
	if isReviewer {
		config.ReviewerReminderInterval = interval
		c.String(200, fmt.Sprintf("レビュワー割り当て後のリマインド頻度を %d分 に設定しました。", interval))
	} else {
		config.ReminderInterval = interval
		c.String(200, fmt.Sprintf("レビュワー募集中のリマインド頻度を %d分 に設定しました。", interval))
	}

	config.UpdatedAt = time.Now()
	db.Save(&config)
}

// setBusinessHoursStart sets the business hours start time
func setBusinessHoursStart(c *gin.Context, db *gorm.DB, channelID, labelName, startTime string) {
	if !isValidTimeFormat(startTime) {
		c.String(200, "時間形式が無効です。HH:MM形式で指定してください（例: 09:00）")
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
		c.String(200, fmt.Sprintf("ラベル「%s」の営業開始時間を %s に設定しました。", labelName, startTime))
		return
	}

	config.BusinessHoursStart = startTime
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の営業開始時間を %s に更新しました。", labelName, startTime))
}

// setBusinessHoursEnd sets the business hours end time
func setBusinessHoursEnd(c *gin.Context, db *gorm.DB, channelID, labelName, endTime string) {
	if !isValidTimeFormat(endTime) {
		c.String(200, "時間形式が無効です。HH:MM形式で指定してください（例: 18:00）")
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
		c.String(200, fmt.Sprintf("ラベル「%s」の営業終了時間を %s に設定しました。", labelName, endTime))
		return
	}

	// Update existing config
	config.BusinessHoursEnd = endTime
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の営業終了時間を %s に更新しました。", labelName, endTime))
}

// setTimezone sets the timezone
func setTimezone(c *gin.Context, db *gorm.DB, channelID, labelName, timezone string) {
	// Validate the timezone
	if !isValidTimezone(timezone) {
		c.String(200, "無効なタイムゾーンです。例: Asia/Tokyo, UTC, America/New_York")
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
		c.String(200, fmt.Sprintf("ラベル「%s」のタイムゾーンを %s に設定しました。", labelName, timezone))
		return
	}

	// Update existing config
	config.Timezone = timezone
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」のタイムゾーンを %s に更新しました。", labelName, timezone))
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

func mapUser(c *gin.Context, db *gorm.DB, params string) {
	parts := strings.Fields(params)
	if len(parts) < 2 {
		c.String(200, "使用方法: /slack-review-notify map-user <github-username> @slack-user\n例: /slack-review-notify map-user octocat @user")
		return
	}

	githubUsername := strings.TrimSpace(parts[0])
	slackUserID := cleanUserID(parts[1])

	if githubUsername == "" || slackUserID == "" {
		c.String(200, "GitHubユーザー名とSlackユーザーIDの両方を指定してください。")
		return
	}

	var existingMapping models.UserMapping
	result := db.Where("github_username = ?", githubUsername).First(&existingMapping)

	if result.Error == nil {
		existingMapping.SlackUserID = slackUserID
		existingMapping.UpdatedAt = time.Now()
		db.Save(&existingMapping)
		c.String(200, fmt.Sprintf("GitHubユーザー `%s` のマッピングを <@%s> に更新しました。", githubUsername, slackUserID))
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
		c.String(200, "ユーザーマッピングの作成に失敗しました。")
		return
	}

	c.String(200, fmt.Sprintf("GitHubユーザー `%s` を <@%s> にマッピングしました。", githubUsername, slackUserID))
}

func showUserMappings(c *gin.Context, db *gorm.DB) {
	var mappings []models.UserMapping

	if err := db.Find(&mappings).Error; err != nil {
		log.Printf("failed to get user mappings: %v", err)
		c.String(200, "ユーザーマッピングの取得に失敗しました。")
		return
	}

	if len(mappings) == 0 {
		c.String(200, "まだユーザーマッピングが登録されていません。\n/slack-review-notify map-user コマンドで登録してください。")
		return
	}

	response := "*登録済みのユーザーマッピング*\n"
	for _, mapping := range mappings {
		response += fmt.Sprintf("• GitHub: `%s` → Slack: <@%s>\n", mapping.GithubUsername, mapping.SlackUserID)
	}

	c.String(200, response)
}

func removeUserMapping(c *gin.Context, db *gorm.DB, githubUsername string) {
	githubUsername = strings.TrimSpace(githubUsername)

	if githubUsername == "" {
		c.String(200, "使用方法: /slack-review-notify remove-user-mapping <github-username>\n例: /slack-review-notify remove-user-mapping octocat")
		return
	}

	var mapping models.UserMapping
	result := db.Where("github_username = ?", githubUsername).First(&mapping)

	if result.Error != nil {
		c.String(200, fmt.Sprintf("GitHubユーザー `%s` のマッピングは存在しません。", githubUsername))
		return
	}

	if err := db.Delete(&mapping).Error; err != nil {
		log.Printf("failed to delete user mapping: %v", err)
		c.String(200, "ユーザーマッピングの削除に失敗しました。")
		return
	}

	c.String(200, fmt.Sprintf("GitHubユーザー `%s` のマッピングを削除しました。", githubUsername))
}

// setAway marks a user as away/on leave
func setAway(c *gin.Context, db *gorm.DB, channelID, labelName, params string) {
	if params == "" {
		c.String(200, "休暇に設定するユーザーを指定してください。例: /slack-review-notify set-away @user [until YYYY-MM-DD] [reason 理由]")
		return
	}

	parts := strings.Fields(params)
	if len(parts) == 0 {
		c.String(200, "休暇に設定するユーザーを指定してください。")
		return
	}

	slackUserID := cleanUserID(parts[0])
	if slackUserID == "" {
		c.String(200, "有効なユーザーIDを指定してください。")
		return
	}

	var awayUntil *time.Time
	var reason string

	// Parse parameters: "until" and "reason" keywords
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "until":
			if i+1 < len(parts) {
				i++
				parsed, err := time.Parse("2006-01-02", parts[i])
				if err != nil {
					c.String(200, "日付形式が無効です。YYYY-MM-DD形式で指定してください（例: 2025-06-01）")
					return
				}
				// Set to end of the specified day (23:59:59)
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
					loc = time.UTC
				}
				endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc)
				if endOfDay.Before(time.Now().In(loc)) {
					c.String(200, "過去の日付は指定できません。今日以降の日付を指定してください。")
					return
				}
				awayUntil = &endOfDay
			}
		case "reason":
			if i+1 < len(parts) {
				reason = strings.Join(parts[i+1:], " ")
				i = len(parts) // End loop
			}
		}
	}

	// Update if existing record exists, otherwise create new (upsert)
	var existing models.ReviewerAvailability
	result := db.Where("slack_user_id = ?", slackUserID).First(&existing)
	if result.Error == nil {
		existing.AwayUntil = awayUntil
		existing.Reason = reason
		existing.UpdatedAt = time.Now()
		if err := db.Save(&existing).Error; err != nil {
			log.Printf("failed to update reviewer availability: %v", err)
			c.String(200, "休暇設定の更新に失敗しました。")
			return
		}
	} else {
		record := models.ReviewerAvailability{
			ID:          uuid.NewString(),
			SlackUserID: slackUserID,
			AwayUntil:   awayUntil,
			Reason:      reason,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := db.Create(&record).Error; err != nil {
			log.Printf("failed to create reviewer availability: %v", err)
			c.String(200, "休暇設定の作成に失敗しました。")
			return
		}
	}

	// Build response message
	response := fmt.Sprintf("<@%s> を休暇に設定しました", slackUserID)
	if awayUntil != nil {
		response += fmt.Sprintf("（%s まで", awayUntil.Format("2006-01-02"))
	} else {
		response += "（無期限"
	}
	if reason != "" {
		response += fmt.Sprintf("、理由: %s）", reason)
	} else {
		response += "）"
	}

	c.String(200, response)
}

// unsetAway removes a user's away/leave status
func unsetAway(c *gin.Context, db *gorm.DB, params string) {
	if params == "" {
		c.String(200, "休暇を解除するユーザーを指定してください。例: /slack-review-notify unset-away @user")
		return
	}

	slackUserID := cleanUserID(strings.TrimSpace(params))
	if slackUserID == "" {
		c.String(200, "有効なユーザーIDを指定してください。")
		return
	}

	result := db.Unscoped().Where("slack_user_id = ?", slackUserID).Delete(&models.ReviewerAvailability{})
	if result.RowsAffected == 0 {
		c.String(200, fmt.Sprintf("<@%s> は休暇に設定されていません。", slackUserID))
		return
	}

	c.String(200, fmt.Sprintf("<@%s> の休暇を解除しました", slackUserID))
}

// showAvailability displays a list of users currently on leave
func showAvailability(c *gin.Context, db *gorm.DB) {
	var records []models.ReviewerAvailability
	now := time.Now()

	// Get records where AwayUntil is nil (indefinite) or in the future
	db.Where("away_until IS NULL OR away_until > ?", now).Find(&records)

	if len(records) == 0 {
		c.String(200, "現在休暇中のユーザーはいません")
		return
	}

	response := "*現在休暇中のユーザー*\n"
	for _, r := range records {
		line := fmt.Sprintf("• <@%s> - ", r.SlackUserID)
		if r.AwayUntil != nil {
			line += fmt.Sprintf("%s まで", r.AwayUntil.Format("2006-01-02"))
		} else {
			line += "無期限"
		}
		if r.Reason != "" {
			line += fmt.Sprintf("（理由: %s）", r.Reason)
		}
		response += line + "\n"
	}

	c.String(200, response)
}

// setRequiredApprovals sets the number of required approvals
func setRequiredApprovals(c *gin.Context, db *gorm.DB, channelID, labelName, countStr string) {
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 || count > 10 {
		c.String(200, "必要なapprove数は1〜10の整数で指定してください。")
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
		c.String(200, fmt.Sprintf("ラベル「%s」の必要なapprove数を %d に設定しました。", labelName, count))
		return
	}

	config.RequiredApprovals = count
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の必要なapprove数を %d に更新しました。", labelName, count))
}
