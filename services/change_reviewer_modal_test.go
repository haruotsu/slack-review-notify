package services

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"slack-review-notify/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func changeReviewerValues(target, newUser string) map[string]map[string]ViewStateValue {
	values := map[string]map[string]ViewStateValue{
		ChangeReviewerNewBlockID: {
			ChangeReviewerNewBlockID: {Type: "users_select", SelectedUser: newUser},
		},
	}
	targetValue := ViewStateValue{Type: "radio_buttons"}
	if target != "" {
		targetValue.SelectedOption = &ViewSelectedOption{Value: target}
	}
	values[ChangeReviewerTargetBlockID] = map[string]ViewStateValue{ChangeReviewerTargetBlockID: targetValue}
	return values
}

func multiReviewerTask() models.ReviewTask {
	return models.ReviewTask{
		ID:              "task-1",
		Title:           "Add feature",
		PRURL:           "https://github.com/o/r/pull/1",
		Reviewer:        "UAAA",
		Reviewers:       "UAAA,UBBB",
		PRAuthorSlackID: "UAUTHOR",
		Language:        "ja",
	}
}

func TestUsesChangeReviewerModal(t *testing.T) {
	tests := []struct {
		name string
		task models.ReviewTask
		want bool
	}{
		{"2人以上なら true", models.ReviewTask{Reviewers: "U1,U2"}, true},
		{"1人でも true", models.ReviewTask{Reviewers: "U1"}, true},
		{"Reviewers 未設定の旧タスクも Reviewer がいれば true", models.ReviewTask{Reviewer: "U1"}, true},
		{"レビュワー不在なら交代対象がいないので false", models.ReviewTask{}, false},
		{"空要素だけなら false", models.ReviewTask{Reviewers: " , ,"}, false},
		{"radio_buttons の上限10人までは true", models.ReviewTask{Reviewers: "U1,U2,U3,U4,U5,U6,U7,U8,U9,U10"}, true},
		{"上限を超えるとモーダルを描画できないので false", models.ReviewTask{Reviewers: "U1,U2,U3,U4,U5,U6,U7,U8,U9,U10,U11"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UsesChangeReviewerModal(tt.task))
		})
	}
}

func TestReplaceTaskReviewer(t *testing.T) {
	t.Run("指定したレビュワーだけが置き換わり順序は保たれる", func(t *testing.T) {
		task := models.ReviewTask{Reviewer: "UAAA", Reviewers: "UAAA,UBBB,UCCC"}
		ReplaceTaskReviewer(&task, "UBBB", "UNEW")
		assert.Equal(t, "UAAA,UNEW,UCCC", task.Reviewers)
		assert.Equal(t, "UAAA", task.Reviewer, "Reviewer は置き換え対象でなければ変わらない")
	})

	t.Run("Reviewer と一致する人を置き換えると Reviewer も追従する", func(t *testing.T) {
		task := models.ReviewTask{Reviewer: "UAAA", Reviewers: "UAAA,UBBB"}
		ReplaceTaskReviewer(&task, "UAAA", "UNEW")
		assert.Equal(t, "UNEW,UBBB", task.Reviewers)
		assert.Equal(t, "UNEW", task.Reviewer)
	})

	t.Run("Reviewers 未設定の旧タスクは Reviewer が置き換わる", func(t *testing.T) {
		task := models.ReviewTask{Reviewer: "UAAA"}
		ReplaceTaskReviewer(&task, "UAAA", "UNEW")
		assert.Equal(t, "", task.Reviewers)
		assert.Equal(t, "UNEW", task.Reviewer)
	})
}

func TestBuildChangeReviewerModalView(t *testing.T) {
	view := BuildChangeReviewerModalView(multiReviewerTask())

	assert.Equal(t, ChangeReviewerModalCallbackID, view["callback_id"])

	meta, err := DecodeChangeReviewerModalMetadata(view["private_metadata"].(string))
	require.NoError(t, err)
	assert.Equal(t, "task-1", meta.TaskID)

	blocks := view["blocks"].([]map[string]any)
	elementOf := func(blockID string) map[string]any {
		for _, b := range blocks {
			if b["block_id"] == blockID {
				return b["element"].(map[string]any)
			}
		}
		t.Fatalf("block %s not found", blockID)
		return nil
	}

	target := elementOf(ChangeReviewerTargetBlockID)
	assert.Equal(t, "radio_buttons", target["type"])
	options := target["options"].([]map[string]any)
	require.Len(t, options, 2)
	assert.Equal(t, "UAAA", options[0]["value"])
	assert.Equal(t, map[string]any{"type": "mrkdwn", "text": "<@UAAA>"}, options[0]["text"],
		"選択肢はメンションとして名前表示されるよう mrkdwn であること")
	assert.Equal(t, "UBBB", options[1]["value"])

	assert.Equal(t, "users_select", elementOf(ChangeReviewerNewBlockID)["type"])

	b, err := json.Marshal(view)
	require.NoError(t, err)
	assert.Contains(t, string(b), "レビュワー変更")
}

func TestBuildChangeReviewerModalView_EscapesTitle(t *testing.T) {
	task := multiReviewerTask()
	task.Title = "Fix <script> & <!channel>"

	view := BuildChangeReviewerModalView(task)

	header := view["blocks"].([]map[string]any)[0]["text"].(map[string]any)["text"].(string)
	assert.Contains(t, header, "Fix &lt;script&gt; &amp; &lt;!channel&gt;")
	assert.NotContains(t, header, "<!channel>", "PRタイトル経由でメンションが発火しないこと")
}

func TestParseChangeReviewerModalSubmission(t *testing.T) {
	task := multiReviewerTask()

	t.Run("妥当な入力をパースできる", func(t *testing.T) {
		form, err := ParseChangeReviewerModalSubmission(changeReviewerValues("UBBB", "UNEW1"), task, ChangeReviewerModalMetadata{TaskID: task.ID})
		require.NoError(t, err)
		assert.Equal(t, "UBBB", form.OldReviewerID)
		assert.Equal(t, "UNEW1", form.NewReviewerID)
	})

	t.Run("後任が空欄ならランダム抽選を意味する空文字で返る", func(t *testing.T) {
		form, err := ParseChangeReviewerModalSubmission(changeReviewerValues("UBBB", ""), task, ChangeReviewerModalMetadata{TaskID: task.ID})
		require.NoError(t, err)
		assert.Equal(t, "UBBB", form.OldReviewerID)
		assert.Equal(t, "", form.NewReviewerID)
	})

	errCases := []struct {
		name    string
		target  string
		newUser string
		blockID string
	}{
		{"交代対象が未選択", "", "UNEW1", ChangeReviewerTargetBlockID},
		{"交代対象が既に担当から外れている", "UGONE", "UNEW1", ChangeReviewerTargetBlockID},
		{"後任が解決済みのユーザーIDでない", "UBBB", "someone", ChangeReviewerNewBlockID},
		{"後任がPR作成者", "UBBB", "UAUTHOR", ChangeReviewerNewBlockID},
		{"後任が既に担当中のレビュワー", "UBBB", "UAAA", ChangeReviewerNewBlockID},
		{"後任が交代対象本人", "UBBB", "UBBB", ChangeReviewerNewBlockID},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseChangeReviewerModalSubmission(changeReviewerValues(tc.target, tc.newUser), task, ChangeReviewerModalMetadata{TaskID: task.ID})
			var ve *ModalValidationError
			require.True(t, errors.As(err, &ve), "ModalValidationError が返ること: %v", err)
			assert.Contains(t, ve.Errors, tc.blockID)
		})
	}
}

func TestTaskReviewerIDs_DropsDuplicates(t *testing.T) {
	assert.Equal(t, []string{"U1", "U2"}, TaskReviewerIDs(models.ReviewTask{Reviewers: "U1,U2,U1"}))
}

func TestBuildChangeReviewerModalView_SingleReviewer(t *testing.T) {
	task := multiReviewerTask()
	task.Reviewers = "UAAA"

	view := BuildChangeReviewerModalView(task)

	meta, err := DecodeChangeReviewerModalMetadata(view["private_metadata"].(string))
	require.NoError(t, err)
	assert.Equal(t, "UAAA", meta.FixedTarget, "交代対象は選ばせずメタデータで運ぶ")

	var hasTargetInput, hasNewInput bool
	var texts []string
	for _, b := range view["blocks"].([]map[string]any) {
		switch b["block_id"] {
		case ChangeReviewerTargetBlockID:
			hasTargetInput = true
		case ChangeReviewerNewBlockID:
			hasNewInput = true
		}
		if txt, ok := b["text"].(map[string]any); ok {
			texts = append(texts, txt["text"].(string))
		}
	}
	assert.False(t, hasTargetInput, "1人なら交代対象の選択欄は出さない")
	assert.True(t, hasNewInput)
	assert.Contains(t, strings.Join(texts, "\n"), "<@UAAA>", "交代対象は表示だけする")
}

func TestParseChangeReviewerModalSubmission_FixedTarget(t *testing.T) {
	task := multiReviewerTask()
	task.Reviewers = "UAAA"
	meta := ChangeReviewerModalMetadata{TaskID: task.ID, FixedTarget: "UAAA"}
	values := map[string]map[string]ViewStateValue{
		ChangeReviewerNewBlockID: {ChangeReviewerNewBlockID: {Type: "users_select", SelectedUser: "UNEW1"}},
	}

	t.Run("メタデータの交代対象を使う", func(t *testing.T) {
		form, err := ParseChangeReviewerModalSubmission(values, task, meta)
		require.NoError(t, err)
		assert.Equal(t, "UAAA", form.OldReviewerID)
		assert.Equal(t, "UNEW1", form.NewReviewerID)
	})

	t.Run("交代対象が既に外れていたら後任欄にエラーを出す", func(t *testing.T) {
		stale := ChangeReviewerModalMetadata{TaskID: task.ID, FixedTarget: "UGONE"}
		_, err := ParseChangeReviewerModalSubmission(values, task, stale)
		var ve *ModalValidationError
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, map[string]string{ChangeReviewerNewBlockID: "このレビュワーは既に担当から外れています。モーダルを開き直してください。"}, ve.Errors)
	})
}
