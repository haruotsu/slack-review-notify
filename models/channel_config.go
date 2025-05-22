package models

import (
	"time"

	"gorm.io/gorm"
)

// ChannelConfig はチャンネル単位の設定を管理するモデル
// 同じチャンネル内でのラベル名は一意であるべき制約を持つ
type ChannelConfig struct {
	ID                       string `gorm:"primaryKey"`
	SlackChannelID           string `gorm:"uniqueIndex:idx_channel_label"` // チャンネルID（名前空間）
	LabelName                string `gorm:"uniqueIndex:idx_channel_label"` // 通知対象ラベル名（チャンネル内で一意）
	DefaultMentionID         string // デフォルトのメンション先（ユーザーID）
	ReviewerList             string // レビュワーリスト（カンマ区切り）
	RepositoryList           string // 通知対象リポジトリのリスト（カンマ区切り）
	IsActive                 bool   // 有効/無効フラグ
	ReminderInterval         int    // リマインド頻度（分単位、デフォルト30分）
	ReviewerReminderInterval int    // レビュワー割り当て後のリマインド頻度（分単位、デフォルト30分）
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}
