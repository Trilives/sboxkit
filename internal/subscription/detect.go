package subscription

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

type SourceKind string

const (
	SourceClash   SourceKind = "clash"
	SourceSingBox SourceKind = "sing-box"
	SourceBase64  SourceKind = "base64"
)

func Detect(content []byte) (SourceKind, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return "", errors.New("empty subscription content")
	}

	if looksLikeSingBox(trimmed) {
		return SourceSingBox, nil
	}
	if looksLikeClash(trimmed) {
		return SourceClash, nil
	}
	if looksLikeBase64(trimmed) {
		return SourceBase64, nil
	}

	return "", errors.New("unknown subscription source")
}

func looksLikeSingBox(content []byte) bool {
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return false
	}
	_, ok := data["outbounds"]
	return ok
}

func looksLikeClash(content []byte) bool {
	text := string(content)
	return strings.Contains(text, "proxies:")
}

func looksLikeBase64(content []byte) bool {
	text := strings.TrimSpace(string(content))
	if text == "" {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(padBase64(text))
	return err == nil
}

func padBase64(text string) string {
	if rem := len(text) % 4; rem != 0 {
		text += strings.Repeat("=", 4-rem)
	}
	return text
}
