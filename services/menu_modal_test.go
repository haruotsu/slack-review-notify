package services

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func menuViewBlocks(t *testing.T, view map[string]any) []map[string]any {
	t.Helper()
	blocks, ok := view["blocks"].([]map[string]any)
	require.True(t, ok, "view must carry blocks")
	return blocks
}

func menuButtonActionIDs(t *testing.T, view map[string]any) []string {
	t.Helper()
	var ids []string
	for _, block := range menuViewBlocks(t, view) {
		elements, _ := block["elements"].([]map[string]any)
		for _, el := range elements {
			ids = append(ids, el["action_id"].(string))
		}
	}
	return ids
}

func TestBuildMenuModalView_OffersEveryOperationAsAButton(t *testing.T) {
	view := BuildMenuModalView(MenuModalMetadata{ChannelID: "C1", UserID: "U1"}, "ja")

	var want []string
	for _, op := range append(append([]string{}, MenuShowOps...), MenuConfigOps...) {
		want = append(want, MenuActionPrefix+op)
	}
	assert.Equal(t, want, menuButtonActionIDs(t, view))
}

func TestBuildMenuModalView_HasNoSubmitBecauseButtonsDoTheWork(t *testing.T) {
	view := BuildMenuModalView(MenuModalMetadata{ChannelID: "C1", UserID: "U1"}, "ja")

	assert.NotContains(t, view, "submit")
	assert.Equal(t, MenuModalCallbackID, view["callback_id"])
}

func TestBuildMenuModalView_CarriesTheChannelItWasOpenedFrom(t *testing.T) {
	view := BuildMenuModalView(MenuModalMetadata{ChannelID: "C1", UserID: "U1"}, "ja")

	meta, err := DecodeMenuModalMetadata(view["private_metadata"].(string))
	require.NoError(t, err)
	assert.Equal(t, MenuModalMetadata{ChannelID: "C1", UserID: "U1"}, meta)
}

// Slack rejects the whole view when a title or button label is over its limit,
// and every operation label doubles as the title of the view it opens.
func TestMenuTexts_FitSlackTitleAndButtonLimits(t *testing.T) {
	for _, lang := range []string{"ja", "en"} {
		t.Run(lang, func(t *testing.T) {
			view := BuildMenuModalView(MenuModalMetadata{}, lang)
			title := view["title"].(map[string]any)["text"].(string)
			assert.LessOrEqual(t, utf8.RuneCountInString(title), SlackModalTitleLimit, "menu title")

			for _, op := range append(append([]string{}, MenuShowOps...), MenuConfigOps...) {
				label := MenuOpLabel(op, lang)
				assert.NotEqual(t, "modal.menu.op."+op, label, "operation %q has no label", op)
				assert.LessOrEqual(t, utf8.RuneCountInString(label), SlackModalTitleLimit, "label of %q", op)
			}
		})
	}
}

func TestBuildMenuResultView_SplitsTextLongerThanOneSection(t *testing.T) {
	line := strings.Repeat("a", 100)
	long := strings.TrimSuffix(strings.Repeat(line+"\n", 50), "\n")

	view := BuildMenuResultView(MenuModalMetadata{ChannelID: "C1"}, "ja", "title", []string{long})

	blocks := menuViewBlocks(t, view)
	assert.Greater(t, len(blocks), 1)
	for _, block := range blocks {
		text := block["text"].(map[string]any)["text"].(string)
		assert.LessOrEqual(t, utf8.RuneCountInString(text), SlackSectionTextLimit)
	}
}

func TestBuildMenuResultView_SeparatesEntriesAndSkipsEmptyOnes(t *testing.T) {
	view := BuildMenuResultView(MenuModalMetadata{}, "ja", "title", []string{"first", "", "second"})

	var types []string
	for _, block := range menuViewBlocks(t, view) {
		types = append(types, block["type"].(string))
	}
	assert.Equal(t, []string{"section", "divider", "section"}, types)
}

func TestBuildMenuResultView_StaysWithinSlackBlockLimit(t *testing.T) {
	texts := make([]string, 80)
	for i := range texts {
		texts[i] = fmt.Sprintf("entry %d", i)
	}

	view := BuildMenuResultView(MenuModalMetadata{}, "ja", "title", texts)

	blocks := menuViewBlocks(t, view)
	assert.Len(t, blocks, SlackModalMaxBlocks)
	assert.Equal(t, "表示しきれない分は省略しました。", blocks[len(blocks)-1]["text"].(map[string]any)["text"],
		"cutting silently would pass off a partial list as the whole one")
}

func TestBuildMenuResultView_NeverEmitsAnEmptySection(t *testing.T) {
	text := strings.Repeat("a", SlackSectionTextLimit) + "\n"

	view := BuildMenuResultView(MenuModalMetadata{}, "ja", "title", []string{text})

	for _, block := range menuViewBlocks(t, view) {
		assert.NotEmpty(t, strings.TrimSpace(block["text"].(map[string]any)["text"].(string)))
	}
}

func TestBuildMenuAvailabilityView_ShowsEveryPeriodWhenTheyExactlyFit(t *testing.T) {
	rows := make([]MenuAvailabilityRow, SlackModalMaxBlocks-1)
	for i := range rows {
		rows[i] = MenuAvailabilityRow{ID: fmt.Sprintf("leave-%d", i), Text: "row"}
	}

	view := BuildMenuAvailabilityView(MenuModalMetadata{}, "ja", rows)

	blocks := menuViewBlocks(t, view)
	require.Len(t, blocks, SlackModalMaxBlocks)
	assert.Contains(t, blocks[len(blocks)-1], "accessory", "the last block must be a period, not a truncation note")
}

func TestRenameLabelBlockID_DiffersPerLabel(t *testing.T) {
	assert.NotEqual(t, RenameLabelBlockID("bug"), RenameLabelBlockID("needs-review"))
	assert.Equal(t, RenameLabelBlockID("bug"), RenameLabelBlockID("bug"))
}

func TestBuildMenuAvailabilityView_RowButtonRemovesThatPeriodAfterConfirming(t *testing.T) {
	view := BuildMenuAvailabilityView(MenuModalMetadata{ChannelID: "C1"}, "ja", []MenuAvailabilityRow{
		{ID: "leave-1", Text: "<@U1> 2026-12-01"},
		{ID: "leave-2", Text: "<@U2> 2026-12-02"},
	})

	blocks := menuViewBlocks(t, view)
	require.Len(t, blocks, 3)
	for i, id := range []string{"leave-1", "leave-2"} {
		accessory := blocks[i+1]["accessory"].(map[string]interface{})
		assert.Equal(t, MenuUnsetAwayActionID, accessory["action_id"])
		assert.Equal(t, id, accessory["value"])
		assert.Contains(t, accessory, "confirm", "an irreversible removal must ask first")
	}
}

func TestBuildMenuAvailabilityView_SaysSoWhenNobodyIsAway(t *testing.T) {
	view := BuildMenuAvailabilityView(MenuModalMetadata{}, "ja", nil)

	blocks := menuViewBlocks(t, view)
	require.Len(t, blocks, 1)
	assert.NotContains(t, blocks[0], "accessory")
	assert.Equal(t, "現在休暇中・予約中のユーザーはいません", blocks[0]["text"].(map[string]any)["text"])
}

func TestBuildMenuAvailabilityView_StaysWithinSlackBlockLimit(t *testing.T) {
	rows := make([]MenuAvailabilityRow, 150)
	for i := range rows {
		rows[i] = MenuAvailabilityRow{ID: fmt.Sprintf("leave-%d", i), Text: "row"}
	}

	view := BuildMenuAvailabilityView(MenuModalMetadata{}, "ja", rows)

	blocks := menuViewBlocks(t, view)
	assert.LessOrEqual(t, len(blocks), SlackModalMaxBlocks)
	last := blocks[len(blocks)-1]["text"].(map[string]any)["text"].(string)
	assert.Contains(t, last, "52", "the note must say how many periods were left out")
}
