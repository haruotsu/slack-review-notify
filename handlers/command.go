package handlers

import (
	"fmt"
	"log"
	"slack-review-notify/models"
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
				DefaultMentionID: userID, // コマンド実行者をデフォルトに
				LabelName:        "needs-review",
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
		
		// /review-config コマンドの処理
		if command == "/review-config" {
			parts := strings.Split(text, " ")
			
			if len(parts) == 0 || parts[0] == "" || parts[0] == "help" {
				// ヘルプを表示
				showHelp(c)
				return
			}
			
			switch parts[0] {
			case "show":
				// 現在の設定を表示
				showConfig(c, db, channelID)
				
			case "set-mention":
				if len(parts) < 2 {
					c.String(200, "メンション先のユーザーIDを指定してください。例: /review-config set-mention U12345678")
					return
				}
				mentionID := parts[1]
				setMention(c, db, channelID, mentionID)
				
			case "add-repo":
				if len(parts) < 2 {
					c.String(200, "リポジトリ名を指定してください。例: /review-config add-repo owner/repo")
					return
				}
				repoName := parts[1]
				addRepository(c, db, channelID, repoName)
				
			case "remove-repo":
				if len(parts) < 2 {
					c.String(200, "リポジトリ名を指定してください。例: /review-config remove-repo owner/repo")
					return
				}
				repoName := parts[1]
				removeRepository(c, db, channelID, repoName)
				
			case "set-label":
				if len(parts) < 2 {
					c.String(200, "ラベル名を指定してください。例: /review-config set-label needs-review")
					return
				}
				labelName := parts[1]
				setLabel(c, db, channelID, labelName)
				
			case "activate":
				activateChannel(c, db, channelID, true)
				
			case "deactivate":
				activateChannel(c, db, channelID, false)
				
			default:
				c.String(200, "不明なコマンドです。/review-config help で使い方を確認してください。")
			}
			
			return
		}
		
		c.String(200, "不明なコマンドです。")
	}
}

// ヘルプメッセージを表示
func showHelp(c *gin.Context) {
	help := `*Review通知Bot設定コマンド*
- /review-config show: 現在の設定を表示
- /review-config set-mention U12345678: メンション先を設定
- /review-config add-repo owner/repo: 監視対象リポジトリを追加
- /review-config remove-repo owner/repo: 監視対象リポジトリを削除
- /review-config set-label label-name: 監視対象ラベルを設定
- /review-config activate: このチャンネルでの通知を有効化
- /review-config deactivate: このチャンネルでの通知を無効化`
	
	c.String(200, help)
}

// 設定を表示
func showConfig(c *gin.Context, db *gorm.DB, channelID string) {
	var config models.ChannelConfig
	
	err := db.Where("slack_channel_id = ?", channelID).First(&config).Error
	if err != nil {
		c.String(200, "このチャンネルの設定はまだありません。/review-config set-mention コマンドで設定を開始してください。")
		return
	}
	
	status := "無効"
	if config.IsActive {
		status = "有効"
	}
	
	response := fmt.Sprintf(`*このチャンネルのレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- 監視対象リポジトリ: %s
- 監視対象ラベル: %s`, 
		status, config.DefaultMentionID, config.RepositoryList, config.LabelName)
	
	c.String(200, response)
}

// メンション先を設定
func setMention(c *gin.Context, db *gorm.DB, channelID, mentionID string) {
	var config models.ChannelConfig
	
	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 新規作成
		config = models.ChannelConfig{
			ID:              uuid.NewString(),
			SlackChannelID:  channelID,
			DefaultMentionID: mentionID,
			LabelName:       "needs-review", // デフォルト値
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("メンション先を <@%s> に設定しました。", mentionID))
		return
	}
	
	// 既存設定を更新
	config.DefaultMentionID = mentionID
	config.UpdatedAt = time.Now()
	db.Save(&config)
	
	c.String(200, fmt.Sprintf("メンション先を <@%s> に更新しました。", mentionID))
}

// リポジトリを追加
func addRepository(c *gin.Context, db *gorm.DB, channelID, repoName string) {
	var config models.ChannelConfig
	
	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:              uuid.NewString(),
			SlackChannelID:  channelID,
			DefaultMentionID: "", // 空のままなのでset-mentionで設定が必要
			RepositoryList:  repoName,
			LabelName:       "needs-review", // デフォルト値
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("監視対象リポジトリに `%s` を追加しました。", repoName))
		return
	}
	
	// 既存のリポジトリリストをチェック
	repos := []string{}
	if config.RepositoryList != "" {
		repos = strings.Split(config.RepositoryList, ",")
		
		// 既に含まれているかチェック
		for _, r := range repos {
			if strings.TrimSpace(r) == repoName {
				c.String(200, fmt.Sprintf("リポジトリ `%s` は既に監視対象です。", repoName))
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
	
	c.String(200, fmt.Sprintf("監視対象リポジトリに `%s` を追加しました。", repoName))
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
		c.String(200, "監視対象リポジトリは設定されていません。")
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
		c.String(200, fmt.Sprintf("リポジトリ `%s` は監視対象ではありません。", repoName))
		return
	}
	
	// 新しいリストを保存
	config.RepositoryList = strings.Join(newRepos, ",")
	config.UpdatedAt = time.Now()
	db.Save(&config)
	
	c.String(200, fmt.Sprintf("監視対象リポジトリから `%s` を削除しました。", repoName))
}

// ラベルを設定
func setLabel(c *gin.Context, db *gorm.DB, channelID, labelName string) {
	var config models.ChannelConfig
	
	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		// 設定がまだない場合は新規作成
		config = models.ChannelConfig{
			ID:              uuid.NewString(),
			SlackChannelID:  channelID,
			DefaultMentionID: "",
			LabelName:       labelName,
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		db.Create(&config)
		c.String(200, fmt.Sprintf("監視対象ラベルを `%s` に設定しました。", labelName))
		return
	}
	
	// 既存の設定を更新
	config.LabelName = labelName
	config.UpdatedAt = time.Now()
	db.Save(&config)
	
	c.String(200, fmt.Sprintf("監視対象ラベルを `%s` に更新しました。", labelName))
}

// チャンネルの有効/無効を切り替え
func activateChannel(c *gin.Context, db *gorm.DB, channelID string, active bool) {
	var config models.ChannelConfig
	
	result := db.Where("slack_channel_id = ?", channelID).First(&config)
	if result.Error != nil {
		if active {
			c.String(200, "このチャンネルの設定はまだありません。/review-config set-mention コマンドで設定を開始してください。")
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
