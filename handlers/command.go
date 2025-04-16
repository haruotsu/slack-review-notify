package handlers

import (
	"fmt"
	"log"
	"net/http"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Slackのスラッシュコマンドを処理するハンドラ
func HandleSlackCommand(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		command := c.PostForm("command")
		text := c.PostForm("text")
		channelID := c.PostForm("channel_id")
		userID := c.PostForm("user_id")
		
		log.Printf("slack command received: command=%s, text=%s, channel=%s, user=%s",
			command, text, channelID, userID)
		
		args := strings.Fields(text)
		if len(args) > 0 && args[0] == "bye" {
			leaveChannel(c, db, channelID)
			return
		}

		// 論理削除されたものも含めてチャンネル設定を検索
		var config models.ChannelConfig
		// Unscoped() で論理削除されたレコードも検索対象にする
		result := db.Unscoped().Where("slack_channel_id = ?", channelID).First(&config)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// レコードが存在しない場合は新規作成
				log.Printf("channel config not found for %s, creating new one.", channelID)
				newConfig := models.ChannelConfig{
					ID:               uuid.NewString(),
					SlackChannelID:   channelID,
					DefaultMentionID: userID,
					LabelName:        "needs-review",
					IsActive:         true, // 新規作成時は有効
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
				}
				// Create を使用して新規作成
				createResult := db.Create(&newConfig)
				if createResult.Error != nil {
					log.Printf("channel config create error: %v", createResult.Error)
					c.String(200, "チャンネル設定の作成に失敗しました。管理者に連絡してください。")
					return
				}
				config = newConfig // 作成した設定を使用
				log.Printf("successfully created new channel config for %s", channelID)
			} else {
				// その他のDBエラー
				log.Printf("db error finding channel config for %s: %v", channelID, result.Error)
				c.String(http.StatusInternalServerError, "チャンネル設定の取得中にエラーが発生しました。")
				return
			}
		} else {
			// レコードが見つかった場合
			if config.DeletedAt.Valid {
				// 論理削除されていた場合は復元 (is_active を true にし、deleted_at を NULL に)
				log.Printf("found logically deleted channel config for %s, restoring...", channelID)
				// config.DeletedAt = gorm.DeletedAt{} // deleted_at を NULL に設定
				config.IsActive = true             // 念のため is_active を true に
				config.UpdatedAt = time.Now()
				// Unscoped を使って更新し、deleted_at を NULL にする
				// updateResult := db.Unscoped().Model(&config).Updates(map[string]interface{}{"deleted_at": nil, "is_active": true, "updated_at": time.Now()})

				// GORM v2の推奨: Clauses(clause.Returning{}) を使うか、直接 NULL をセットする
				// updateResult := db.Unscoped().Model(&config).UpdateColumns(map[string]interface{}{"deleted_at": gorm.DeletedAt{}, "is_active": true, "updated_at": time.Now()})
				// gorm.DeletedAt{} がうまく NULL にならない場合があるため、明示的に NULL を指定
				updateResult := db.Unscoped().Model(&config).Updates(map[string]interface{}{"deleted_at": nil, "is_active": true, "updated_at": time.Now()})


				if updateResult.Error != nil {
					log.Printf("failed to restore channel config for %s: %v", channelID, updateResult.Error)
					c.String(http.StatusInternalServerError, "チャンネル設定の復元に失敗しました。")
					return
				}
				// 更新後のデータを再読み込み (必須ではないが、最新の状態を保証するため)
				// db.Unscoped().Where("slack_channel_id = ?", channelID).First(&config)
				log.Printf("successfully restored channel config for %s", channelID)
			} else {
				// 既に存在し、論理削除されていない場合 (特に何もしない)
				log.Printf("found active channel config for %s", channelID)
			}
		}
		
		// /slack-review-notify コマンドの処理 (bye以外)
		if command == "/slack-review-notify" {
			if len(args) == 0 {
				showHelp(c)
				return
			}

			action := args[0]
			value := ""
			if len(args) > 1 {
				value = strings.Join(args[1:], " ")
			}

			switch action {
			case "show":
				showConfig(c, config)
			case "set-mention":
				setMention(c, db, config, value)
			case "add-repo":
				addRepo(c, db, config, value)
			case "remove-repo":
				removeRepo(c, db, config, value)
			case "set-label":
				setLabel(c, db, config, value)
			case "add-reviewer":
				addReviewer(c, db, config, value)
			case "remove-reviewer":
				removeReviewer(c, db, config, value)
			case "activate":
				// 既に有効な場合は何もしない、無効な場合は有効にする
				if !config.IsActive {
					toggleActivation(c, db, config, true)
				} else {
					c.String(200, "通知は既に有効です。")
				}
			case "deactivate":
				// 既に無効な場合は何もしない、有効な場合は無効にする
				if config.IsActive {
					toggleActivation(c, db, config, false)
				} else {
					c.String(200, "通知は既に無効です。")
				}
			case "help":
				showHelp(c)
			default:
				c.String(200, fmt.Sprintf("不明なコマンドです: %s\n`/slack-review-notify help` で利用可能なコマンドを確認できます。", action))
			}
			
			return
		}
		
		c.String(200, "不明なコマンドです。")
	}
}

// ヘルプメッセージを表示
func showHelp(c *gin.Context) {
	helpText := `*利用可能なコマンド:*

• /slack-review-notify show - 現在の設定を表示
• /slack-review-notify set-mention @username - メンション先を設定
• /slack-review-notify add-repo owner/repo - 監視するリポジトリを追加
• /slack-review-notify remove-repo owner/repo - 監視するリポジトリを削除
• /slack-review-notify set-label needs-review - 通知対象のラベルを設定
• /slack-review-notify add-reviewer UXXXXXXXX,UYYYYYYYY - レビュワー候補を追加
• /slack-review-notify remove-reviewer UXXXXXXXX - レビュワー候補を削除
• /slack-review-notify activate - 通知を有効化
• /slack-review-notify deactivate - 通知を無効化
• /slack-review-notify help - このヘルプメッセージを表示

詳細は README を参照してください。`

	c.String(200, helpText)
}

// 設定を表示
func showConfig(c *gin.Context, config models.ChannelConfig) {
	status := "無効"
	if config.IsActive {
		status = "有効"
	}

	// レビュワーリストをメンション形式に変換
	reviewerListStr := "未設定"
	if config.ReviewerList != "" {
		reviewerIDs := strings.Split(config.ReviewerList, ",")
		reviewerMentions := []string{}
		for _, id := range reviewerIDs {
			trimmedID := strings.TrimSpace(id)
			if trimmedID != "" {
				// IDが空でない場合のみメンション形式にする
				reviewerMentions = append(reviewerMentions, fmt.Sprintf("<@%s>", trimmedID))
			}
		}
		if len(reviewerMentions) > 0 {
			reviewerListStr = strings.Join(reviewerMentions, ", ")
		}
	}

	response := fmt.Sprintf(`*このチャンネルのレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- 通知対象リポジトリ: %s
- 通知対象ラベル: %s
- レビュワー候補: %s`,
		status, config.DefaultMentionID, config.RepositoryList, config.LabelName, reviewerListStr) // 修正: reviewerListStrを使用

	c.String(200, response)
}

// メンション先を設定
func setMention(c *gin.Context, db *gorm.DB, config models.ChannelConfig, mentionID string) {
	if mentionID == "" {
		c.String(200, "メンション先のSlack User IDを指定してください。\n例: `/slack-review-notify set-mention UXXXXXXXX`")
		return
	}

	// メンションIDからユーザーID部分を抽出（<@UserID> or @username形式）
	cleanedID := strings.TrimSpace(mentionID)
	if strings.HasPrefix(cleanedID, "<@") && strings.HasSuffix(cleanedID, ">") {
		cleanedID = strings.TrimPrefix(cleanedID, "<@")
		cleanedID = strings.TrimSuffix(cleanedID, ">")
	} else if strings.HasPrefix(cleanedID, "@") {
		// @username形式の場合、@を除去 (ただし、UserIDが望ましい)
		// 本来はここでSlack APIを叩くなどしてUserIDに変換するのが理想
		cleanedID = strings.TrimPrefix(cleanedID, "@")
		// 注意: ここではUserID形式(Uから始まる)かどうかのチェックは省略
	}

	// UserIDとして妥当か簡易チェック（例：空でないこと）
	if cleanedID == "" {
		c.String(200, "無効なメンション先が指定されました。")
		return
	}

	config.DefaultMentionID = cleanedID // UserID (or username) のみを保存
	config.UpdatedAt = time.Now()

	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save mention ID for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "メンション先の保存に失敗しました。")
		return
	}

	c.String(200, fmt.Sprintf("メンション先を <@%s> に設定しました。", config.DefaultMentionID)) // 表示時に<@...>で囲む
}

// リポジトリを追加
func addRepo(c *gin.Context, db *gorm.DB, config models.ChannelConfig, repoName string) {
	if repoName == "" {
		c.String(200, "追加するリポジトリを指定してください。\n例: `/slack-review-notify add-repo owner/repo`")
		return
	}

	// 既存のリポジトリリストをチェック
	repos := []string{}
	if config.RepositoryList != "" {
		repos = strings.Split(config.RepositoryList, ",")
		
		// 既に含まれているかチェック
		for _, r := range repos {
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
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save repository for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "リポジトリの追加に失敗しました。")
		return
	}
	
	c.String(200, fmt.Sprintf("通知対象リポジトリに `%s` を追加しました。", repoName))
}

// リポジトリを削除
func removeRepo(c *gin.Context, db *gorm.DB, config models.ChannelConfig, repoName string) {
	if repoName == "" {
		c.String(200, "削除するリポジトリを指定してください。\n例: `/slack-review-notify remove-repo owner/repo`")
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
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save repository list for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "リポジトリの削除に失敗しました。")
		return
	}
	
	c.String(200, fmt.Sprintf("通知対象リポジトリから `%s` を削除しました。", repoName))
}

// ラベルを設定
func setLabel(c *gin.Context, db *gorm.DB, config models.ChannelConfig, labelName string) {
	if labelName == "" {
		c.String(200, "設定するラベル名を指定してください。\n例: `/slack-review-notify set-label needs-review`")
		return
	}
	
	config.LabelName = labelName
	config.UpdatedAt = time.Now()
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save label for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "ラベルの設定に失敗しました。")
		return
	}
	
	c.String(200, fmt.Sprintf("通知対象ラベルを `%s` に設定しました。", labelName))
}

// レビュアーを追加
func addReviewer(c *gin.Context, db *gorm.DB, config models.ChannelConfig, value string) {
	if value == "" {
		c.String(200, "追加するレビュアーのSlack User IDをカンマ区切りで指定してください。\n例: `/slack-review-notify add-reviewer UXXXXXXXX,UYYYYYYYY`")
		return
	}

	newReviewersRaw := strings.Split(value, ",")
	existingReviewers := []string{}
	if config.ReviewerList != "" {
		existingReviewers = strings.Split(config.ReviewerList, ",")
	}

	addedCount := 0
	currentReviewerMap := make(map[string]bool)
	for _, r := range existingReviewers {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			currentReviewerMap[trimmed] = true
		}
	}

	updatedReviewers := existingReviewers
	addedMentions := []string{} // 追加されたレビュアーのメンション表示用

	for _, nrRaw := range newReviewersRaw {
		trimmed := strings.TrimSpace(nrRaw)
		cleanedID := ""
		// <@UserID> または @username 形式から UserID または username を抽出
		if strings.HasPrefix(trimmed, "<@") && strings.HasSuffix(trimmed, ">") {
			cleanedID = strings.TrimPrefix(trimmed, "<@")
			cleanedID = strings.TrimSuffix(cleanedID, ">")
		} else if strings.HasPrefix(trimmed, "@") {
			cleanedID = strings.TrimPrefix(trimmed, "@")
			// 注意: UserID形式かのチェックは省略
		} else if trimmed != "" {
			// その他の形式（UserID直接など）
			cleanedID = trimmed
		}

		if cleanedID != "" && !currentReviewerMap[cleanedID] {
			updatedReviewers = append(updatedReviewers, cleanedID) // UserID (or username) をリストに追加
			currentReviewerMap[cleanedID] = true
			addedMentions = append(addedMentions, fmt.Sprintf("<@%s>", cleanedID)) // 表示用にメンション形式も保持
			addedCount++
		}
	}

	// 空要素を除去 (念のため)
	finalReviewers := []string{}
	finalMentions := []string{} // 最終的なリストのメンション表示用
	for _, r := range updatedReviewers {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			finalReviewers = append(finalReviewers, trimmed)
			finalMentions = append(finalMentions, fmt.Sprintf("<@%s>", trimmed))
		}
	}

	config.ReviewerList = strings.Join(finalReviewers, ",") // DBにはID (or username) のみを保存
	config.UpdatedAt = time.Now()
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save reviewer list for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "レビュアーリストの保存に失敗しました。")
		return
	}

	if addedCount > 0 {
		// 応答メッセージではメンション形式で表示
		c.String(200, fmt.Sprintf("%d名のレビュアー (%s) を追加しました。\n現在のレビュアーリスト: %s",
			addedCount, strings.Join(addedMentions, ", "), strings.Join(finalMentions, ", ")))
	} else {
		c.String(200, fmt.Sprintf("指定されたレビュアーは既に追加済みか、無効なIDです。\n現在のレビュアーリスト: %s",
			strings.Join(finalMentions, ", ")))
	}
}

// レビュアーを削除
func removeReviewer(c *gin.Context, db *gorm.DB, config models.ChannelConfig, value string) {
	if value == "" {
		c.String(200, "削除するレビュアーのSlack User IDをカンマ区切りで指定してください。\n例: `/slack-review-notify remove-reviewer UXXXXXXXX,UYYYYYYYY`")
		return
	}
	if config.ReviewerList == "" {
		c.String(200, "レビュアーリストは現在空です。")
		return
	}

	toRemoveReviewersRaw := strings.Split(value, ",")
	existingReviewers := strings.Split(config.ReviewerList, ",")

	removeMap := make(map[string]bool)
	removedMentions := []string{} // 削除されるレビュアーのメンション表示用

	for _, rRaw := range toRemoveReviewersRaw {
		trimmed := strings.TrimSpace(rRaw)
		cleanedID := ""
		// <@UserID> または @username 形式から UserID または username を抽出
		if strings.HasPrefix(trimmed, "<@") && strings.HasSuffix(trimmed, ">") {
			cleanedID = strings.TrimPrefix(trimmed, "<@")
			cleanedID = strings.TrimSuffix(trimmed, ">")
		} else if strings.HasPrefix(trimmed, "@") {
			cleanedID = strings.TrimPrefix(trimmed, "@")
			// 注意: UserID形式かのチェックは省略
		} else if trimmed != "" {
			cleanedID = trimmed
		}

		if cleanedID != "" {
			removeMap[cleanedID] = true
			// 削除対象が見つかった場合に表示用メンションを追加（削除前に）
			// （実際にリストに存在するかは後でチェックするが、ユーザー指定の表示のため）
			removedMentions = append(removedMentions, fmt.Sprintf("<@%s>", cleanedID))
		}
	}

	updatedReviewers := []string{}
	removedCount := 0
	finalMentions := []string{} // 最終的なリストのメンション表示用

	for _, er := range existingReviewers {
		trimmed := strings.TrimSpace(er)
		if trimmed != "" && !removeMap[trimmed] {
			updatedReviewers = append(updatedReviewers, trimmed)
			finalMentions = append(finalMentions, fmt.Sprintf("<@%s>", trimmed))
		} else if trimmed != "" && removeMap[trimmed] {
			// 実際に削除された場合にカウント
			removedCount++
		} else if trimmed != "" {
             // 削除対象外だが、最終リストには含める
             finalMentions = append(finalMentions, fmt.Sprintf("<@%s>", trimmed))
        }
	}


	// 削除対象がリストに存在しなかった場合のエッジケース対応
	if removedCount == 0 && len(removeMap) > 0 {
         // ユーザーは削除を指定したが、リストに該当者がいなかった
         currentMentions := []string{}
         for _, r := range existingReviewers {
              trimmed := strings.TrimSpace(r)
              if trimmed != "" {
                   currentMentions = append(currentMentions, fmt.Sprintf("<@%s>", trimmed))
              }
         }
         c.String(200, fmt.Sprintf("指定されたレビュアー (%s) は見つかりませんでした。\n現在のレビュアーリスト: %s",
              strings.Join(removedMentions, ", "), strings.Join(currentMentions, ", ")))
         return
    }


	config.ReviewerList = strings.Join(updatedReviewers, ",") // IDのみ保存
	config.UpdatedAt = time.Now()
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save reviewer list for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "レビュアーリストの保存に失敗しました。")
		return
	}

	// 応答メッセージではメンション形式で表示
	c.String(200, fmt.Sprintf("%d名のレビュアー (%s) を削除しました。\n現在のレビュアーリスト: %s",
		removedCount, strings.Join(removedMentions, ", "), strings.Join(finalMentions, ", ")))

}

// 通知の有効/無効を切り替え
func toggleActivation(c *gin.Context, db *gorm.DB, config models.ChannelConfig, active bool) {
	config.IsActive = active
	config.UpdatedAt = time.Now()
	if err := db.Save(&config).Error; err != nil {
		log.Printf("Failed to save activation status for channel %s: %v", config.SlackChannelID, err)
		c.String(http.StatusInternalServerError, "通知設定の変更に失敗しました。")
		return
	}
	
	if active {
		c.String(200, "このチャンネルでのレビュー通知を有効化しました。")
	} else {
		c.String(200, "このチャンネルでのレビュー通知を無効化しました。")
	}
}

// チャンネルから退出する
func leaveChannel(c *gin.Context, db *gorm.DB, channelID string) {
	log.Printf("Attempting to leave channel: %s", channelID)

	// ★★★ 退出メッセージを投稿 ★★★
	if err := services.PostLeaveMessage(channelID); err != nil {
		// メッセージ投稿失敗はログに残すが、退出処理は続行
		log.Printf("Failed to post leave message to channel %s (proceeding with leave): %v", channelID, err)
	}

	// 実際にチャンネルから退出
	err := services.LeaveSlackChannel(channelID)
	if err != nil {
		log.Printf("Failed to leave channel %s: %v", channelID, err)
		// エラーメッセージをユーザーに返す（APIエラーの詳細を含む方が親切かも）
		c.String(200, fmt.Sprintf("チャンネルからの退出に失敗しました: %s", err.Error()))
		return
	}

	log.Printf("Successfully left channel: %s", channelID)

	// ★★★ チャンネル設定を削除する処理を追加 ★★★
	if err := db.Where("slack_channel_id = ?", channelID).Delete(&models.ChannelConfig{}).Error; err != nil {
		log.Printf("Failed to delete channel config for %s: %v", channelID, err)
		// チャンネル退出自体は成功しているので、ここではエラーを返さない (ログのみ)
	} else {
		log.Printf("Successfully deleted channel config for %s", channelID)
	}
	// ★★★ ここまで追加 ★★★

	// Slackはコマンド応答を期待しているので、空のOKを返すのが一般的
	c.Status(http.StatusOK)
}
