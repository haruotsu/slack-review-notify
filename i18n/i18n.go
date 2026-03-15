package i18n

import (
	"fmt"
	"os"
	"sync"
)

var (
	currentLang = "ja"
	langMu      sync.RWMutex
)

var messages = map[string]map[string]string{
	"ja": messagesJa,
	"en": messagesEn,
}

// Init reads the language setting from the LANGUAGE environment variable.
func Init() {
	lang := os.Getenv("LANGUAGE")
	if lang == "" {
		lang = "ja"
	}
	SetLang(lang)
}

// SetLang sets the current language.
func SetLang(lang string) {
	langMu.Lock()
	defer langMu.Unlock()
	currentLang = lang
}

// T returns the translated string using the global language setting.
func T(key string, args ...interface{}) string {
	langMu.RLock()
	lang := currentLang
	langMu.RUnlock()
	return TWithLang(lang, key, args...)
}

// TWithLang returns the translated string for a specific language.
// If lang is empty, falls back to "ja".
func TWithLang(lang, key string, args ...interface{}) string {
	if lang == "" {
		lang = "ja"
	}

	msgs, ok := messages[lang]
	if !ok {
		msgs = messagesJa
	}

	format, ok := msgs[key]
	if !ok {
		return key
	}

	if len(args) == 0 {
		return format
	}

	return fmt.Sprintf(format, args...)
}

// L returns a translator function bound to the specified language.
// Usage: t := i18n.L("en"); t("cmd.unknown")
func L(lang string) func(string, ...interface{}) string {
	return func(key string, args ...interface{}) string {
		return TWithLang(lang, key, args...)
	}
}
