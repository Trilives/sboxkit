package compat

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// Report 「生成器 + 内核 + 规则集 + 生效订阅」组合的校验结论。新旧版本 sboxkit 之间经
// `sboxkit compat-check --report` 以 JSON 交换，字段只增不改。
type Report struct {
	Generator    string `json:"generator"`              // 生成器（sboxkit）版本
	MinKernel    string `json:"min_kernel"`             // 生成器要求的最低 sing-box 版本
	Kernel       string `json:"kernel"`                 // 校验所用内核版本
	Subscription string `json:"subscription,omitempty"` // 生效订阅；空表示没有可重新生成的订阅
	Compatible   bool   `json:"compatible"`
	Error        string `json:"error,omitempty"`      // 不兼容：内核拒绝的输出或版本要求
	Hint         string `json:"hint,omitempty"`       // 处理方向
	Unverified   string `json:"unverified,omitempty"` // 无法重新生成（如缺少订阅原文）的原因
}

// Explain 面向用户的结论说明（不兼容时含所需版本与影响）。
func (r Report) Explain() string {
	if r.Compatible || r.Error == "" {
		return ""
	}
	msg := fmt.Sprintf(i18n.T("sboxkit %s（要求 sing-box ≥ %s）为生效订阅「%s」生成的配置不被 sing-box %s 接受：%s"),
		r.Generator, r.MinKernel, r.Subscription, orUnknown(r.Kernel), r.Error)
	if r.Hint != "" {
		msg += "\n" + r.Hint
	}
	return msg + "\n" + i18n.T("影响：按这一组合，刷新、重建或切换订阅都会被拒绝，无法用上新生成的配置。")
}

// NeedsNewerKernel 校验所用内核低于生成器要求的最低版本：补救方向是更新内核。
func (r Report) NeedsNewerKernel() bool {
	ok, known := AtLeast(r.Kernel, r.MinKernel)
	return known && !ok
}

// WriteReport 以 0600 写入 JSON 报告。
func WriteReport(path string, r Report) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// ReadReport 读取 JSON 报告。
func ReadReport(path string) (Report, error) {
	var r Report
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("parse compat report %s: %w", path, err)
	}
	return r, nil
}

func orUnknown(version string) string {
	if version == "" {
		return "?"
	}
	return version
}
