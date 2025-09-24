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

// Slackのスラッシュコマンドを処理するハンドラ
func HandleSlackCommand(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		// ボディを復元
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 署名を検証
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

		// すべてのチャンネル設定を出力してデバッグ
		var allConfigs []models.ChannelConfig
		db.Find(&allConfigs)
		log.Printf("all channel configs in database (%d):", len(allConfigs))
		for i, cfg := range allConfigs {
			log.Printf("[%d] ID=%s, Channel=%s, Label=%s", i, cfg.ID, cfg.SlackChannelID, cfg.LabelName)
		}

		// /slack-review-notify コマンドの処理
		if command == "/slack-review-notify" {
			// コマンド部分とパラメータを分離
			var labelName, subCommand, params string

			// テキストをクォート対応で分割
			parts := parseCommand(text)
			
			if len(parts) == 0 {
				// 引数がない場合はヘルプを表示
				showHelp(c)
				return
			}
			
			// 最初の引数がサブコマンドかラベル名か判断
			potentialSubCommands := []string{"show", "help", "set-mention", "add-reviewer", 
				"show-reviewers", "clear-reviewers", "add-repo", "remove-repo", 
				"set-label", "activate", "deactivate", "set-reviewer-reminder-interval",
				"set-business-hours-start", "set-business-hours-end", "set-timezone"}
			
			isSubCommand := false
			for _, cmd := range potentialSubCommands {
				if parts[0] == cmd {
					isSubCommand = true
					break
				}
			}
			
			if isSubCommand {
				// 最初の引数がサブコマンドの場合、デフォルトのラベル名を使用
				subCommand = parts[0]
				labelName = "needs-review" // デフォルトのラベル名
				
				if len(parts) > 1 {
					params = strings.Join(parts[1:], " ")
				}
			} else {
				// 最初の引数がラベル名の場合
				labelName = parts[0]
				
				if len(parts) > 1 {
					subCommand = parts[1]
					
					if len(parts) > 2 {
						params = strings.Join(parts[2:], " ")
					}
				} else {
					// ラベル名のみが指定された場合はその設定を表示
					subCommand = "show"
				}
			}

			// ラベル名に関するロギング
			log.Printf("command parsed: label=%s, subCommand=%s, params=%s", 
				labelName, subCommand, params)

			if subCommand == "" || subCommand == "help" {
				// ヘルプを表示
				showHelp(c)
				return
			}

			// ラベル名を指定して既存設定を取得
			var config models.ChannelConfig
			result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
			
			// ラベル名を指定して設定が見つからなかった場合は新しく作成
			if result.Error != nil {
				log.Printf("config for channel(%s) and label(%s) not found: %v", 
					channelID, labelName, result.Error)
				
				// 新しい設定の作成は各コマンド処理内で行う
			}

			switch subCommand {
			case "show":
				// 現在の設定を表示
				if isSubCommand && len(parts) == 1 {
					// ラベル名が指定されていない場合はすべてのラベルを表示
					showAllLabels(c, db, channelID)
				} else {
					// 特定のラベルの設定を表示
					showConfig(c, db, channelID, labelName)
				}

			case "set-mention":
				if params == "" {
					c.String(200, "メンション先のユーザーIDを指定してください。例: /slack-review-notify " + labelName + " set-mention @user")
					return
				}
				mentionID := strings.TrimSpace(params)
				setMention(c, db, channelID, labelName, mentionID)

			case "add-reviewer":
				if params == "" {
					c.String(200, "レビュワーのユーザーIDをカンマ区切りで指定してください。例: /slack-review-notify " + labelName + " add-reviewer @user1,@user2")
					return
				}
				// 正規表現を使ってすべてのスペースパターンを処理
				re := regexp.MustCompile(`\s*,\s*`)
				reviewerIDs := re.ReplaceAllString(params, ",")

				// 先頭と末尾の空白も削除
				reviewerIDs = strings.TrimSpace(reviewerIDs)
				addReviewers(c, db, channelID, labelName, reviewerIDs)

			case "show-reviewers":
				// レビュワーリストを表示
				showReviewers(c, db, channelID, labelName)

			case "clear-reviewers":
				// レビュワーリストをクリア
				clearReviewers(c, db, channelID, labelName)

			case "add-repo":
				if params == "" {
					c.String(200, "リポジトリ名をカンマ区切りで指定してください。例: /slack-review-notify " + labelName + " add-repo owner/repo1,owner/repo2")
					return
				}
				repoName := params
				addRepository(c, db, channelID, labelName, repoName)

			case "remove-repo":
				if params == "" {
					c.String(200, "リポジトリ名を指定してください。例: /slack-review-notify " + labelName + " remove-repo owner/repo")
					return
				}
				repoName := params
				removeRepository(c, db, channelID, labelName, repoName)

			case "set-label":
				// set-label は実際にはラベル名を変更する操作に変更
				if params == "" {
					c.String(200, "新しいラベル名を指定してください。例: /slack-review-notify " + labelName + " set-label new-label-name")
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
					c.String(200, "レビュワー割り当て後のリマインド頻度を分単位で指定してください。例: /slack-review-notify " + labelName + " set-reviewer-reminder-interval 30")
					return
				}
				setReminderInterval(c, db, channelID, labelName, strings.TrimSpace(params), true)

			case "set-business-hours-start":
				if params == "" {
					c.String(200, "営業開始時間を指定してください。例: /slack-review-notify " + labelName + " set-business-hours-start 09:00")
					return
				}
				setBusinessHoursStart(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-business-hours-end":
				if params == "" {
					c.String(200, "営業終了時間を指定してください。例: /slack-review-notify " + labelName + " set-business-hours-end 18:00")
					return
				}
				setBusinessHoursEnd(c, db, channelID, labelName, strings.TrimSpace(params))

			case "set-timezone":
				if params == "" {
					c.String(200, "タイムゾーンを指定してください。例: /slack-review-notify " + labelName + " set-timezone Asia/Tokyo")
					return
				}
				setTimezone(c, db, channelID, labelName, strings.TrimSpace(params))

			default:
				c.String(200, "不明なコマンドです。/slack-review-notify help で使い方を確認してください。")
			}

			return
		}

		c.String(200, "不明なコマンドです。")
	}
}

// parseCommand はコマンドテキストをクォート対応で解析する
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
				// クォート開始
				inQuote = true
				quoteChar = char
			} else if char == quoteChar {
				// クォート終了
				inQuote = false
				quoteChar = 0
			} else {
				// 異なるクォート文字は普通の文字として扱う
				current.WriteByte(char)
			}
		case char == ' ' && !inQuote:
			// スペースで分割（クォート内でない場合のみ）
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(char)
		}
	}
	
	// 最後の部分を追加
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	
	return parts
}

// ヘルプメッセージを表示
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
• /slack-review-notify [ラベル名] activate - 通知を有効化
• /slack-review-notify [ラベル名] deactivate - 通知を無効化

[ラベル名]を省略すると「needs-review」というデフォルトのラベルを使用します`

	c.String(200, help)
}

// すべてのラベル設定を表示
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

// 設定を表示
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
		timezone = "Asia/Tokyo" // デフォルトタイムゾーン
	}

	response := fmt.Sprintf(`*このチャンネルのラベル「%s」のレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- レビュワーリスト: %s
- 通知対象リポジトリ: %s
- レビュワー割り当て後のリマインド頻度: %d分
- 営業時間: %s - %s (%s)`,
		labelName, status, config.DefaultMentionID, formatReviewerList(config.ReviewerList),
		config.RepositoryList, reviewerReminderInterval, config.BusinessHoursStart, config.BusinessHoursEnd, timezone)

	c.String(200, response)
}

// レビュワーリストをフォーマット
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

// 文字列を清潔なユーザーIDに変換する関数
func cleanUserID(userID string) string {
	// 空白を削除
	userID = strings.TrimSpace(userID)

	// チームメンション形式 <!subteam^ID|@name> の処理
	if strings.HasPrefix(userID, "<!subteam^") && strings.Contains(userID, "|") && strings.HasSuffix(userID, ">") {
		parts := strings.Split(userID, "|")
		if len(parts) > 0 {
			id := strings.TrimPrefix(parts[0], "<!subteam^")
			// チームIDをそのまま返す
			return id
		}
	}

	// 通常のユーザーメンション <@ID> の処理
	if strings.HasPrefix(userID, "<@") && strings.HasSuffix(userID, ">") {
		return strings.TrimPrefix(strings.TrimSuffix(userID, ">"), "<@")
	}

	// @から始まる場合は@を削除
	userID = strings.TrimPrefix(userID, "@")

	// カンマが含まれる場合は削除
	userID = strings.ReplaceAll(userID, ",", "")

	return userID
}

// 複数のユーザーIDを整形
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

// レビュワーを追加
func addReviewers(c *gin.Context, db *gorm.DB, channelID, labelName, reviewerIDs string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			ReviewerList:   cleanupUserIDs(reviewerIDs), // ユーザーID形式を整形
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("ラベル「%s」のレビュワーリストを設定しました: %s", labelName, formatReviewerList(config.ReviewerList)))
		return
	}

	// 既存のレビュワーリストをチェック
	currentReviewers := []string{}
	if config.ReviewerList != "" {
		currentReviewers = strings.Split(config.ReviewerList, ",")
		for i, r := range currentReviewers {
			currentReviewers[i] = strings.TrimSpace(r)
		}
	}

	// 新しいレビュワーを追加
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

	// 更新したリストを保存
	config.ReviewerList = strings.Join(currentReviewers, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」のレビュワーリストを更新しました: %s", labelName, formatReviewerList(config.ReviewerList)))
}

// レビュワーリストを表示
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

// レビュワーリストをクリア
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

// メンション先を設定
func setMention(c *gin.Context, db *gorm.DB, channelID, labelName, mentionID string) {
	var config models.ChannelConfig

	// メンションIDを整形
	cleanedMentionID := cleanUserID(mentionID)

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// 新規作成
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

		// チームメンションかどうかを判定して表示を変える
		var mentionDisplay string
		if strings.HasPrefix(mentionID, "<!subteam^") {
			mentionDisplay = fmt.Sprintf("<!subteam^%s>", cleanedMentionID)
		} else {
			mentionDisplay = fmt.Sprintf("<@%s>", cleanedMentionID)
		}

		c.String(200, fmt.Sprintf("ラベル「%s」のメンション先を %s に設定しました。", labelName, mentionDisplay))
		return
	}

	// 既存設定を更新
	config.DefaultMentionID = cleanedMentionID
	config.UpdatedAt = time.Now()
	db.Save(&config)

	// チームメンションかどうかを判定して表示を変える
	var mentionDisplay string
	if strings.HasPrefix(mentionID, "<!subteam^") {
		mentionDisplay = fmt.Sprintf("<!subteam^%s>", cleanedMentionID)
	} else {
		mentionDisplay = fmt.Sprintf("<@%s>", cleanedMentionID)
	}

	c.String(200, fmt.Sprintf("ラベル「%s」のメンション先を %s に更新しました。", labelName, mentionDisplay))
}

// リポジトリを追加
func addRepository(c *gin.Context, db *gorm.DB, channelID, labelName, repoNames string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:               uuid.NewString(),
			SlackChannelID:   channelID,
			LabelName:        labelName,
			RepositoryList:   repoNames,
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("ラベル「%s」の通知対象リポジトリに `%s` を追加しました。", labelName, repoNames))
		return
	}

	// 正規表現を使ってすべてのスペースパターンを処理
	re := regexp.MustCompile(`\s*,\s*`)
	repoNames = re.ReplaceAllString(repoNames, ",")

	// 先頭と末尾の空白も削除
	repoNames = strings.TrimSpace(repoNames)

	// 既存のリポジトリリストをチェック
	currentRepos := []string{}
	if config.RepositoryList != "" {
		currentRepos = strings.Split(config.RepositoryList, ",")
		for i, r := range currentRepos {
			currentRepos[i] = strings.TrimSpace(r)
		}
	}

	// 新しいリポジトリを追加
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

	// 更新したリストを保存
	config.RepositoryList = strings.Join(currentRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	// 応答メッセージを作成
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

// リポジトリを削除
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

	// リポジトリリストを解析
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

	// 新しいリストを保存
	config.RepositoryList = strings.Join(newRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の通知対象リポジトリから `%s` を削除しました。", labelName, repoName))
}

// ラベル名を変更する関数
func changeLabelName(c *gin.Context, db *gorm.DB, channelID, oldLabelName, newLabelName string) {
	var config models.ChannelConfig

	// 現在の設定を取得
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, oldLabelName).First(&config)
	if result.Error != nil {
		c.String(200, fmt.Sprintf("ラベル「%s」の設定はこのチャンネルに存在しません。", oldLabelName))
		return
	}
	
	// 新しいラベル名で既に設定が存在するか確認
	var existingConfig models.ChannelConfig
	existingResult := db.Where("slack_channel_id = ? AND label_name = ?", channelID, newLabelName).First(&existingConfig)
	if existingResult.Error == nil {
		c.String(200, fmt.Sprintf("ラベル「%s」の設定は既に存在します。別のラベル名を指定してください。", newLabelName))
		return
	}

	// ラベル名を更新
	config.LabelName = newLabelName
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル名を「%s」から「%s」に変更しました。", oldLabelName, newLabelName))
}

// チャンネルの有効/無効を切り替え
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

	// 既存の設定を更新
	config.IsActive = active
	config.UpdatedAt = time.Now()
	db.Save(&config)

	if active {
		c.String(200, fmt.Sprintf("このチャンネルでのラベル「%s」のレビュー通知を有効化しました。", labelName))
	} else {
		c.String(200, fmt.Sprintf("このチャンネルでのラベル「%s」のレビュー通知を無効化しました。", labelName))
	}
}

// リマインド頻度を設定
func setReminderInterval(c *gin.Context, db *gorm.DB, channelID, labelName, intervalStr string, isReviewer bool) {
	var config models.ChannelConfig

	// 数値に変換
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		c.String(200, "リマインド頻度は1以上の整数で指定してください。")
		return
	}

	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
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

	// 既存設定を更新
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

// 営業開始時間を設定
func setBusinessHoursStart(c *gin.Context, db *gorm.DB, channelID, labelName, startTime string) {
	if !isValidTimeFormat(startTime) {
		c.String(200, "時間形式が無効です。HH:MM形式で指定してください（例: 09:00）")
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	
	if result.Error != nil {
		// 新規作成
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

// 営業終了時間を設定
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

	// 既存設定を更新
	config.BusinessHoursEnd = endTime
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」の営業終了時間を %s に更新しました。", labelName, endTime))
}

// タイムゾーンを設定
func setTimezone(c *gin.Context, db *gorm.DB, channelID, labelName, timezone string) {
	// タイムゾーンをバリデート
	if !isValidTimezone(timezone) {
		c.String(200, "無効なタイムゾーンです。例: Asia/Tokyo, UTC, America/New_York")
		return
	}

	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&config)
	
	if result.Error != nil {
		// 新規作成
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      labelName,
			Timezone:       timezone,
			BusinessHoursStart: "09:00",
			BusinessHoursEnd:   "18:00",
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("ラベル「%s」のタイムゾーンを %s に設定しました。", labelName, timezone))
		return
	}

	// 既存設定を更新
	config.Timezone = timezone
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("ラベル「%s」のタイムゾーンを %s に更新しました。", labelName, timezone))
}

// タイムゾーンをバリデート
func isValidTimezone(timezone string) bool {
	_, err := time.LoadLocation(timezone)
	return err == nil
}

// 時間形式（HH:MM）をバリデート
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
