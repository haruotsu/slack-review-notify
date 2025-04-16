package models

import (
	"time"

	"gorm.io/gorm"
)

// ChannelConfig はチャンネルごとの設定を保持します
type ChannelConfig struct {
	ID              string `gorm:"primaryKey"`
	SlackChannelID  string `gorm:"uniqueIndex"` // チャンネルIDをユニークインデックスに
	DefaultMentionID string // デフォルトのメンション先（ユーザーID or グループID）
	RepositoryList  string // 通知対象リポジトリのリスト（カンマ区切り）
	LabelName       string // 通知対象ラベル名（デフォルト: "needs-review"）
	ReviewerList    string // レビュアー候補リスト (Slack User IDのカンマ区切り)
	IsActive        bool   // 有効/無効フラグ
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
} 
