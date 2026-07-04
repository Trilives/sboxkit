// Package jsonx JSON 输出助手：2 空格缩进、非 ASCII 不转义、结尾换行
// （与 Python 版 json.dumps(..., indent=2, ensure_ascii=False) 的写盘习惯一致）。
package jsonx

import (
	"bytes"
	"encoding/json"
)

func MarshalPretty(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil // Encode 已带结尾换行
}
