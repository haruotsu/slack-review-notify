package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitTextForSections_ReturnsNilForEmptyText(t *testing.T) {
	if got := SplitTextForSections("", SlackSectionTextLimit); got != nil {
		t.Errorf("SplitTextForSections(\"\") = %v, want nil", got)
	}
}

func TestSplitTextForSections_KeepsShortTextInOneChunk(t *testing.T) {
	text := "line one\nline two"

	got := SplitTextForSections(text, SlackSectionTextLimit)

	if len(got) != 1 || got[0] != text {
		t.Errorf("SplitTextForSections(%q) = %v, want [%q]", text, got, text)
	}
}

func TestSplitTextForSections_SplitsAtLineBoundariesWithoutLosingContent(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = strings.Repeat("あ", 20)
	}
	text := strings.Join(lines, "\n")
	const limit = 100

	got := SplitTextForSections(text, limit)

	if len(got) < 2 {
		t.Fatalf("expected the text to be split, got %d chunk(s)", len(got))
	}
	for i, chunk := range got {
		if n := utf8.RuneCountInString(chunk); n > limit {
			t.Errorf("chunk %d has %d characters, want <= %d", i, n, limit)
		}
		if strings.HasPrefix(chunk, "\n") || strings.HasSuffix(chunk, "\n") {
			t.Errorf("chunk %d should not be padded with newlines: %q", i, chunk)
		}
	}
	if rejoined := strings.Join(got, "\n"); rejoined != text {
		t.Errorf("rejoined text does not match the original")
	}
}

func TestSplitTextForSections_HardSplitsLineLongerThanLimit(t *testing.T) {
	text := strings.Repeat("あ", 250)
	const limit = 100

	got := SplitTextForSections(text, limit)

	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	for i, chunk := range got {
		if n := utf8.RuneCountInString(chunk); n > limit {
			t.Errorf("chunk %d has %d characters, want <= %d", i, n, limit)
		}
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}
	}
	if rejoined := strings.Join(got, ""); rejoined != text {
		t.Errorf("rejoined text does not match the original")
	}
}

func TestTruncateForSlack_LeavesTextWithinLimitUntouched(t *testing.T) {
	text := "short label"

	if got := TruncateForSlack(text, SlackButtonTextLimit); got != text {
		t.Errorf("TruncateForSlack(%q) = %q, want %q", text, got, text)
	}
}

func TestTruncateForSlack_CutsOverlongTextOnRuneBoundary(t *testing.T) {
	text := strings.Repeat("あ", 200)

	got := TruncateForSlack(text, SlackButtonTextLimit)

	if n := utf8.RuneCountInString(got); n != SlackButtonTextLimit {
		t.Errorf("truncated text has %d characters, want %d", n, SlackButtonTextLimit)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text should end with an ellipsis, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Error("truncated text is not valid UTF-8")
	}
}

func TestCreateButton_TruncatesOverlongText(t *testing.T) {
	button := CreateButton(strings.Repeat("x", 200), "action", "value", "")

	textBlock, ok := button["text"].(map[string]interface{})
	if !ok {
		t.Fatalf("button text block has unexpected type %T", button["text"])
	}
	label, ok := textBlock["text"].(string)
	if !ok {
		t.Fatalf("button label has unexpected type %T", textBlock["text"])
	}
	if n := utf8.RuneCountInString(label); n > SlackButtonTextLimit {
		t.Errorf("button label has %d characters, want <= %d", n, SlackButtonTextLimit)
	}
}
