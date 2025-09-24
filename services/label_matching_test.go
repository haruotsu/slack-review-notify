package services

import (
	"slack-review-notify/models"
	"testing"

	"github.com/google/go-github/v71/github"
	"github.com/stretchr/testify/assert"
)

func TestIsLabelMatched(t *testing.T) {
	tests := []struct {
		name           string
		configLabels   string
		prLabels       []*github.Label
		expectedResult bool
	}{
		{
			name:         "単一ラベル設定で単一ラベルPRがマッチ",
			configLabels: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("needs-review")},
			},
			expectedResult: true,
		},
		{
			name:         "単一ラベル設定で複数ラベルPRがマッチ",
			configLabels: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
				{Name: github.Ptr("priority")},
			},
			expectedResult: true,
		},
		{
			name:         "単一ラベル設定でラベルが存在しない",
			configLabels: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("bug")},
				{Name: github.Ptr("feature")},
			},
			expectedResult: false,
		},
		{
			name:         "複数ラベル設定で全ラベルが存在（AND条件）",
			configLabels: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			expectedResult: true,
		},
		{
			name:         "複数ラベル設定で一部ラベルのみ存在",
			configLabels: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("bug")},
			},
			expectedResult: false,
		},
		{
			name:         "カンマ前後のスペースを適切に処理",
			configLabels: "hoge-project , needs-review , bug",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
				{Name: github.Ptr("feature")},
			},
			expectedResult: true,
		},
		{
			name:           "空文字列の場合は全てマッチ（後方互換）",
			configLabels:   "",
			prLabels: []*github.Label{
				{Name: github.Ptr("any-label")},
			},
			expectedResult: true,
		},
		{
			name:           "PRにラベルがない場合",
			configLabels:   "needs-review",
			prLabels:       []*github.Label{},
			expectedResult: false,
		},
		{
			name:           "設定が空でPRにもラベルがない",
			configLabels:   "",
			prLabels:       []*github.Label{},
			expectedResult: true,
		},
		{
			name:         "3つのラベル全てが必要な設定",
			configLabels: "project-a,needs-review,urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("project-a")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")},
				{Name: github.Ptr("bug")},
			},
			expectedResult: true,
		},
		{
			name:         "3つのラベルのうち1つが不足",
			configLabels: "project-a,needs-review,urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("project-a")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.ChannelConfig{
				LabelName: tt.configLabels,
			}

			result := IsLabelMatched(config, tt.prLabels)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetMissingLabels(t *testing.T) {
	tests := []struct {
		name            string
		configLabels    string
		prLabels        []*github.Label
		expectedMissing []string
	}{
		{
			name:         "単一ラベル設定でラベルが存在",
			configLabels: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("needs-review")},
			},
			expectedMissing: []string{},
		},
		{
			name:         "単一ラベル設定でラベルが存在しない",
			configLabels: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("bug")},
			},
			expectedMissing: []string{"needs-review"},
		},
		{
			name:         "複数ラベル設定で全て存在",
			configLabels: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")},
			},
			expectedMissing: []string{},
		},
		{
			name:         "複数ラベル設定で一部のみ存在",
			configLabels: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
			},
			expectedMissing: []string{"needs-review"},
		},
		{
			name:         "複数ラベル設定で複数が不足",
			configLabels: "project-a,needs-review,urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("bug")},
			},
			expectedMissing: []string{"project-a", "needs-review", "urgent"},
		},
		{
			name:            "空の設定",
			configLabels:    "",
			prLabels:        []*github.Label{{Name: github.Ptr("any")}},
			expectedMissing: []string{},
		},
		{
			name:         "スペース付きラベル設定",
			configLabels: "project-a, needs-review , urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("project-a")},
				{Name: github.Ptr("urgent")},
			},
			expectedMissing: []string{"needs-review"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.ChannelConfig{
				LabelName: tt.configLabels,
			}

			result := GetMissingLabels(config, tt.prLabels)
			assert.Equal(t, tt.expectedMissing, result)
		})
	}
}