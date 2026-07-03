package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/paths"
)

type uiLanguage string

const (
	languageEnglish uiLanguage = "en"
	languageChinese uiLanguage = "zh"
)

type uiPreferences struct {
	Language uiLanguage `json:"language"`
}

func loadUIPreferences() uiPreferences {
	path := uiPreferencesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return uiPreferences{Language: languageEnglish}
	}
	var prefs uiPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return uiPreferences{Language: languageEnglish}
	}
	if prefs.Language != languageChinese {
		prefs.Language = languageEnglish
	}
	return prefs
}

func saveUIPreferences(prefs uiPreferences) error {
	if prefs.Language != languageChinese {
		prefs.Language = languageEnglish
	}
	path := uiPreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func uiPreferencesPath() string {
	return filepath.Join(paths.FromRoot("").UIDir, "preferences.json")
}

func label(lang uiLanguage, en string, zh string) string {
	if lang == languageChinese {
		return zh
	}
	return en
}
