package i18n

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestT_ReturnsJapanese_WhenLangIsJa(t *testing.T) {
	SetLang("ja")
	got := T("cmd.unknown")
	want := "不明なコマンドです。"
	if got != want {
		t.Errorf("T(\"cmd.unknown\") = %q, want %q", got, want)
	}
}

func TestT_ReturnsEnglish_WhenLangIsEn(t *testing.T) {
	SetLang("en")
	got := T("cmd.unknown")
	want := "Unknown command."
	if got != want {
		t.Errorf("T(\"cmd.unknown\") = %q, want %q", got, want)
	}
}

func TestT_WithFormatArgs(t *testing.T) {
	SetLang("ja")
	got := T("cmd.set_mention.created", "test-label", "<@U123>")
	want := "ラベル「test-label」のメンション先を <@U123> に設定しました。"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestT_WithFormatArgs_English(t *testing.T) {
	SetLang("en")
	got := T("cmd.set_mention.created", "test-label", "<@U123>")
	want := "Set mention target for label \"test-label\" to <@U123>."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestT_ReturnsKey_WhenKeyNotFound(t *testing.T) {
	SetLang("ja")
	got := T("nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("T(\"nonexistent.key\") = %q, want %q", got, "nonexistent.key")
	}
}

func TestSetLang(t *testing.T) {
	SetLang("en")
	if currentLang != "en" {
		t.Errorf("currentLang = %q, want %q", currentLang, "en")
	}
	SetLang("ja")
	if currentLang != "ja" {
		t.Errorf("currentLang = %q, want %q", currentLang, "ja")
	}
}

func TestInit_ReadsFromEnv(t *testing.T) {
	os.Setenv("LANGUAGE", "en")
	defer os.Unsetenv("LANGUAGE")
	Init()
	if currentLang != "en" {
		t.Errorf("currentLang = %q, want %q", currentLang, "en")
	}
}

func TestInit_DefaultsToJa(t *testing.T) {
	os.Unsetenv("LANGUAGE")
	Init()
	if currentLang != "ja" {
		t.Errorf("currentLang = %q, want %q", currentLang, "ja")
	}
}

func TestTWithLang_Japanese(t *testing.T) {
	got := TWithLang("ja", "cmd.unknown")
	want := "不明なコマンドです。"
	if got != want {
		t.Errorf("TWithLang(\"ja\", \"cmd.unknown\") = %q, want %q", got, want)
	}
}

func TestTWithLang_English(t *testing.T) {
	got := TWithLang("en", "cmd.unknown")
	want := "Unknown command."
	if got != want {
		t.Errorf("TWithLang(\"en\", \"cmd.unknown\") = %q, want %q", got, want)
	}
}

func TestTWithLang_EmptyLangDefaultsToJa(t *testing.T) {
	got := TWithLang("", "cmd.unknown")
	want := "不明なコマンドです。"
	if got != want {
		t.Errorf("TWithLang(\"\", \"cmd.unknown\") = %q, want %q", got, want)
	}
}

func TestTWithLang_WithFormatArgs(t *testing.T) {
	got := TWithLang("en", "cmd.set_mention.created", "my-label", "<@U999>")
	want := "Set mention target for label \"my-label\" to <@U999>."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTWithLang_KeyNotFound(t *testing.T) {
	got := TWithLang("en", "nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("got %q, want %q", got, "nonexistent.key")
	}
}

func TestL_ReturnsTranslatorFunction(t *testing.T) {
	tJa := L("ja")
	tEn := L("en")

	gotJa := tJa("cmd.unknown")
	gotEn := tEn("cmd.unknown")

	if gotJa != "不明なコマンドです。" {
		t.Errorf("L(\"ja\")(\"cmd.unknown\") = %q, want %q", gotJa, "不明なコマンドです。")
	}
	if gotEn != "Unknown command." {
		t.Errorf("L(\"en\")(\"cmd.unknown\") = %q, want %q", gotEn, "Unknown command.")
	}
}

func TestL_WithFormatArgs(t *testing.T) {
	tEn := L("en")
	got := tEn("cmd.set_mention.created", "bug", "<@U1>")
	want := "Set mention target for label \"bug\" to <@U1>."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAllJaKeysExistInEn(t *testing.T) {
	for key := range messagesJa {
		if _, ok := messagesEn[key]; !ok {
			t.Errorf("key %q exists in ja but not in en", key)
		}
	}
}

func TestAllEnKeysExistInJa(t *testing.T) {
	for key := range messagesEn {
		if _, ok := messagesJa[key]; !ok {
			t.Errorf("key %q exists in en but not in ja", key)
		}
	}
}

// formatVerbRe matches a printf verb, skipping "%%" and allowing the explicit
// argument index some messages use to reorder arguments (e.g. "%[2]s %[1]s").
var formatVerbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?([a-zA-Z])`)

// formatVerbs returns the verb letters of s as a sorted multiset, so that a
// translation reordering its arguments still compares equal.
func formatVerbs(s string) []string {
	var verbs []string
	for _, m := range formatVerbRe.FindAllStringSubmatch(strings.ReplaceAll(s, "%%", ""), -1) {
		verbs = append(verbs, m[3])
	}
	sort.Strings(verbs)
	return verbs
}

// TestFormatVerbsMatchAcrossLanguages pins that both translations of a message
// take the same arguments. Callers pass one argument list for every language, so
// a verb dropped from one side renders "%!s(MISSING)" or "%!(EXTRA string=...)"
// to the users of that language only — invisible to anyone exercising the other.
// A verb whose type differs (%s vs %d) misrenders the same way. Key parity alone
// catches neither.
func TestFormatVerbsMatchAcrossLanguages(t *testing.T) {
	for key, ja := range messagesJa {
		en, ok := messagesEn[key]
		if !ok {
			continue // already reported by TestAllJaKeysExistInEn
		}
		jaVerbs, enVerbs := formatVerbs(ja), formatVerbs(en)
		if !slices.Equal(jaVerbs, enVerbs) {
			t.Errorf("key %q takes %v in ja but %v in en\n  ja: %q\n  en: %q",
				key, jaVerbs, enVerbs, ja, en)
		}
	}
}

// slackSectionTextLimit mirrors Slack's per-section text limit. The renderer splits an
// over-long help body across sections rather than letting Slack reject the response, but
// a help text that no longer fits one section has also grown past what anyone reads, so
// it is kept within a single section on purpose.
const slackSectionTextLimit = 3000

func TestHelpTextFitsInOneSlackSection(t *testing.T) {
	for lang := range messages {
		help := TWithLang(lang, "cmd.help")
		if n := utf8.RuneCountInString(help); n > slackSectionTextLimit {
			t.Errorf("cmd.help[%s] has %d characters, want <= %d", lang, n, slackSectionTextLimit)
		}
	}
}
