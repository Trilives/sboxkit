package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/i18n"
)

const DefaultMixedPort = 7890

// EffectiveMixedPort keeps old customize.json files and invalid hand-edited
// values safe: a missing/out-of-range port always falls back to the historical
// default.
func EffectiveMixedPort(cfg Config) int {
	if cfg.MixedPort < 1 || cfg.MixedPort > 65535 {
		return DefaultMixedPort
	}
	return cfg.MixedPort
}

// ParsePort validates an interactive mixed-port value.
func ParsePort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s", i18n.T("端口必须是 1-65535 之间的整数"))
	}
	return port, nil
}
