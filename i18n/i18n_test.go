package i18n

import (
	"os"
	"testing"
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
