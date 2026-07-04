// Package configfile reads sing-box JSON config files (JSON is valid YAML,
// so yaml.v3 parses both the generated sing-box config and any raw Clash
// YAML fixture fed to it).
package configfile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Trilives/sboxkit/internal/i18n"
)

func Parse(raw []byte) (map[string]any, error) {
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("%s", i18n.T("配置根节点不是映射"))
	}
	return cfg, nil
}

func Read(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("解析配置 %s: %w"), path, err)
	}
	return cfg, nil
}
