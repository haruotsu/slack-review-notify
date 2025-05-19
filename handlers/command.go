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
			log.Printf("[%d] ID=%s, Channel=%s", i, cfg.ID, cfg.SlackChannelID)
		}

		// 現在のチャンネルの設定を確認
		var config models.ChannelConfig
		result := db.Where("slack_channel_id = ?", channelID).First(&config)
		if result.Error != nil {
			log.Printf("this channel(%s) config not found: %v", channelID, result.Error)

			// 設定がない場合は自動作成
			newConfig := models.ChannelConfig{
				ID:               uuid.NewString(),
				SlackChannelID:   channelID,
				DefaultMentionID: userID,         // コマンド実行者をデフォルトに
				LabelName:        "needs-review", // デフォルトのラベル名
				IsActive:         true,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			}

			createResult := db.Create(&newConfig)
			if createResult.Error != nil {
				log.Printf("channel config create error: %v", createResult.Error)
				c.String(200, "channel config create failed. please contact the administrator.")
				return
			}
			// 作成した設定を使用
			config = newConfig
		}

		// /slack-review-notify コマンドの処理
		if command == "/slack-review-notify" {
			// コマンド部分とパラメータを分離
			var subCommand, params string

			// 最初の空白でコマンドとパラメータを分割
			parts := strings.SplitN(text, " ", 2)
			subCommand = parts[0]
			if len(parts) > 1 {
				params = parts[1]
			}

			if subCommand == "" || subCommand == "help" {
				// ヘルプを表示
				showHelp(c)
				return
			}

			switch subCommand {
			case "show":
				// 現在の設定を表示
				showConfig(c, db, channelID)

			case "set-mention":
				if params == "" {
					c.String(200, "メンション先のユーザーIDを指定してください。例: /slack-review-notify set-mention @user")
					return
				}
				mentionID := strings.TrimSpace(params)
				setMention(c, db, channelID, mentionID)

			case "add-reviewer":
				if params == "" {
					c.String(200, "レビュワーのユーザーIDをカンマ区切りで指定してください。例: /slack-review-notify add-reviewer @user1,@user2")
					return
				}
				// 正規表現を使ってすべてのスペースパターンを処理
				re := regexp.MustCompile(`\s*,\s*`)
				reviewerIDs := re.ReplaceAllString(params, ",")

				// 先頭と末尾の空白も削除
				reviewerIDs = strings.TrimSpace(reviewerIDs)
				addReviewers(c, db, channelID, reviewerIDs)

			case "show-reviewers":
				// レビュワーリストを表示
				showReviewers(c, db, channelID)

			case "clear-reviewers":
				// レビュワーリストをクリア
				clearReviewers(c, db, channelID)

			case "add-repo":
				if params == "" {
					c.String(200, "リポジトリ名を指定してください。例: /slack-review-notify add-repo owner/repo")
					return
				}
				repoName := params
				addRepository(c, db, channelID, repoName)

			case "remove-repo":
				if params == "" {
					c.String(200, "リポジトリ名を指定してください。例: /slack-review-notify remove-repo owner/repo")
					return
				}
				repoName := params
				removeRepository(c, db, channelID, repoName)

			case "set-label":
				if params == "" {
					c.String(200, "ラベル名を指定してください。例: /slack-review-notify set-label needs-review")
					return
				}
				labelName := params
				setLabel(c, db, channelID, labelName)

			case "activate":
				activateChannel(c, db, channelID, true)

			case "deactivate":
				activateChannel(c, db, channelID, false)

			case "set-reviewer-reminder-interval":
				if params == "" {
					c.String(200, "レビュワー割り当て後のリマインド頻度を分単位で指定してください。例: /slack-review-notify set-reviewer-reminder-interval 30")
					return
				}
				setReminderInterval(c, db, channelID, strings.TrimSpace(params), true)

			default:
				c.String(200, "不明なコマンドです。/slack-review-notify help で使い方を確認してください。")
			}

			return
		}

		c.String(200, "不明なコマンドです。")
	}
}

// ヘルプメッセージを表示
func showHelp(c *gin.Context) {
	help := `*Review通知Bot設定コマンド*
- /slack-review-notify show: 現在の設定を表示
- /slack-review-notify set-mention @user: メンション先を設定
- /slack-review-notify add-reviewer @user1,@user2: レビュワーを追加（カンマ区切り）
- /slack-review-notify show-reviewers: 登録されたレビュワーリストを表示
- /slack-review-notify clear-reviewers: レビュワーリストをクリア
- /slack-review-notify add-repo owner/repo: 通知対象リポジトリを追加
- /slack-review-notify remove-repo owner/repo: 通知対象リポジトリを削除
- /slack-review-notify set-label label-name: 通知対象ラベルを設定
- /slack-review-notify set-reviewer-reminder-interval 30: レビュワー割り当て後のリマインド頻度を設定（分単位）
- /slack-review-notify activate: このチャンネルでの通知を有効化
- /slack-review-notify deactivate: このチャンネルでの通知を無効化`

	c.String(200, help)
}

// 設定を表示
func showConfig(c *gin.Context, db *gorm.DB, channelID string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ?", channelID).First(&config).Error
	if err != nil {
		c.String(200, "このチャンネルの設定はまだありません。/slack-review-notify set-mention コマンドで設定を開始してください。")
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

	response := fmt.Sprintf(`*このチャンネルのレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- レビュワーリスト: %s
- 通知対象リポジトリ: %s
- 通知対象ラベル: %s
- レビュワー割り当て後のリマインド頻度: %d分`,
		status, config.DefaultMentionID, formatReviewerList(config.ReviewerList),
		config.RepositoryList, config.LabelName, reviewerReminderInterval)

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
func addReviewers(c *gin.Context, db *gorm.DB, channelID, reviewerIDs string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			ReviewerList:   cleanupUserIDs(reviewerIDs), // ユーザーID形式を整形
			LabelName:      "needs-review",              // デフォルト値
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("レビュワーリストを設定しました: %s", formatReviewerList(config.ReviewerList)))
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

	c.String(200, fmt.Sprintf("レビュワーリストを更新しました: %s", formatReviewerList(config.ReviewerList)))
}

// レビュワーリストを表示
func showReviewers(c *gin.Context, db *gorm.DB, channelID string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ?", channelID).First(&config).Error
	if err != nil {
		c.String(200, "このチャンネルの設定はまだありません。/slack-review-notify add-reviewer コマンドで設定を開始してください。")
		return
	}

	if config.ReviewerList == "" {
		c.String(200, "現在レビュワーは登録されていません。/slack-review-notify add-reviewer コマンドで追加してください。")
		return
	}

	response := fmt.Sprintf("*このチャンネルのレビュワーリスト*\n%s", formatReviewerList(config.ReviewerList))
	c.String(200, response)
}

// レビュワーリストをクリア
func clearReviewers(c *gin.Context, db *gorm.DB, channelID string) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ?", channelID).First(&config).Error
	if err != nil {
		c.String(200, "このチャンネルの設定はまだありません。")
		return
	}

	config.ReviewerList = ""
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, "レビュワーリストをクリアしました。")
}

// メンション先を設定
func setMention(c *gin.Context, db *gorm.DB, channelID, mentionID string) {
	var config models.ChannelConfig

	// メンションIDを整形
	cleanedMentionID := cleanUserID(mentionID)

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 新規作成
		config = models.ChannelConfig{
			ID:               uuid.NewString(),
			SlackChannelID:   channelID,
			DefaultMentionID: cleanedMentionID,
			LabelName:        "needs-review", // デフォルト値
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

		c.String(200, fmt.Sprintf("メンション先を %s に設定しました。", mentionDisplay))
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

	c.String(200, fmt.Sprintf("メンション先を %s に更新しました。", mentionDisplay))
}

// リポジトリを追加
func addRepository(c *gin.Context, db *gorm.DB, channelID, repoName string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:               uuid.NewString(),
			SlackChannelID:   channelID,
			DefaultMentionID: "", // 空のままなのでset-mentionで設定が必要
			RepositoryList:   repoName,
			LabelName:        "needs-review", // デフォルト値
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("通知対象リポジトリに `%s` を追加しました。", repoName))
		return
	}

	// 既存のリポジトリリストをチェック
	if config.RepositoryList != "" {
		reposList := strings.Split(config.RepositoryList, ",")

		// 既に含まれているかチェック
		for _, r := range reposList {
			if strings.TrimSpace(r) == repoName {
				c.String(200, fmt.Sprintf("リポジトリ `%s` は既に通知対象です。", repoName))
				return
			}
		}
	}

	// リポジトリを追加
	if config.RepositoryList == "" {
		config.RepositoryList = repoName
	} else {
		config.RepositoryList = config.RepositoryList + "," + repoName
	}

	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("通知対象リポジトリに `%s` を追加しました。", repoName))
}

// リポジトリを削除
func removeRepository(c *gin.Context, db *gorm.DB, channelID, repoName string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		c.String(200, "このチャンネルの設定はまだありません。")
		return
	}

	if config.RepositoryList == "" {
		c.String(200, "通知対象リポジトリは設定されていません。")
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
		c.String(200, fmt.Sprintf("リポジトリ `%s` は通知対象ではありません。", repoName))
		return
	}

	// 新しいリストを保存
	config.RepositoryList = strings.Join(newRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("通知対象リポジトリから `%s` を削除しました。", repoName))
}

// ラベルを設定
func setLabel(c *gin.Context, db *gorm.DB, channelID, labelName string) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:               uuid.NewString(),
			SlackChannelID:   channelID,
			DefaultMentionID: "",
			LabelName:        labelName,
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("通知対象ラベルを `%s` に設定しました。", labelName))
		return
	}

	// 既存の設定を更新
	config.LabelName = labelName
	config.UpdatedAt = time.Now()
	db.Save(&config)

	c.String(200, fmt.Sprintf("通知対象ラベルを `%s` に更新しました。", labelName))
}

// チャンネルの有効/無効を切り替え
func activateChannel(c *gin.Context, db *gorm.DB, channelID string, active bool) {
	var config models.ChannelConfig

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		if active {
			c.String(200, "このチャンネルの設定はまだありません。/slack-review-notify set-mention コマンドで設定を開始してください。")
		} else {
			c.String(200, "このチャンネルの設定はまだありません。")
		}
		return
	}

	// 既存の設定を更新
	config.IsActive = active
	config.UpdatedAt = time.Now()
	db.Save(&config)

	if active {
		c.String(200, "このチャンネルでのレビュー通知を有効化しました。")
	} else {
		c.String(200, "このチャンネルでのレビュー通知を無効化しました。")
	}
}

// リマインド頻度を設定
func setReminderInterval(c *gin.Context, db *gorm.DB, channelID, intervalStr string, isReviewer bool) {
	var config models.ChannelConfig

	// 数値に変換
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		c.String(200, "リマインド頻度は1以上の整数で指定してください。")
		return
	}

	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      "needs-review", // デフォルト値
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
