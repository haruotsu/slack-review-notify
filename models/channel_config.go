package models

import (
	"time"

	"gorm.io/gorm"
)

type ChannelConfig struct {
	ID                       string `gorm:"primaryKey"`
	SlackChannelID           string `gorm:"uniqueIndex"` // チャンネルIDをユニークインデックスに
	DefaultMentionID         string // デフォルトのメンション先（ユーザーID）
	ReviewerList             string // レビュワーリスト（カンマ区切り）
	RepositoryList           string // 通知対象リポジトリのリスト（カンマ区切り）
	LabelName                string // 通知対象ラベル名（デフォルト: "needs-review"）
	IsActive                 bool   // 有効/無効フラグ
	ReminderInterval         int    // リマインド頻度（分単位、デフォルト30分）
	ReviewerReminderInterval int    // レビュワー割り当て後のリマインド頻度（分単位、デフォルト30分）
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}
