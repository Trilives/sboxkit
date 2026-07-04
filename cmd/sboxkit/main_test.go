package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestMaybeOfferFirstRunInitPicksLanguageBeforePrompt(t *testing.T) {
	origLang := i18n.Current()
	t.Cleanup(func() { i18n.SetLang(origLang) })
	i18n.SetLang(i18n.EN)

	var calls []string
	deps := firstRunDeps{
		isInstalled: func(name string) bool {
			calls = append(calls, "is-installed:"+name)
			return false
		},
		pickLanguage: func(paths.Paths) error {
			calls = append(calls, "pick-language")
			i18n.SetLang(i18n.ZH)
			return nil
		},
		confirm: func(prompt string, def bool) (bool, error) {
			calls = append(calls, "confirm")
			if prompt != "未检测到已注册的服务，是否现在进行初始化？" {
				t.Fatalf("confirmation prompt should use language chosen first, got %q", prompt)
			}
			if !def {
				t.Fatal("first-run initialization prompt should default to yes")
			}
			return false, nil
		},
		initFlow: func(paths.Paths) error {
			calls = append(calls, "init")
			return nil
		},
		reportError: func(string) {},
	}

	maybeOfferFirstRunInit(paths.Paths{}, deps)

	want := []string{"is-installed:sing-box", "pick-language", "confirm"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("call order mismatch:\nwant %#v\n got %#v", want, calls)
	}
}

func TestMaybeOfferFirstRunInitStopsWhenLanguageSelectionFails(t *testing.T) {
	var calls []string
	deps := firstRunDeps{
		isInstalled: func(string) bool {
			calls = append(calls, "is-installed")
			return false
		},
		pickLanguage: func(paths.Paths) error {
			calls = append(calls, "pick-language")
			return errors.New("save language")
		},
		confirm: func(string, bool) (bool, error) {
			calls = append(calls, "confirm")
			return true, nil
		},
		initFlow: func(paths.Paths) error {
			calls = append(calls, "init")
			return nil
		},
		reportError: func(msg string) {
			calls = append(calls, "error:"+msg)
		},
	}

	maybeOfferFirstRunInit(paths.Paths{}, deps)

	want := []string{"is-installed", "pick-language", "error:save language"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("call order mismatch:\nwant %#v\n got %#v", want, calls)
	}
}
