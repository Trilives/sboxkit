package flows

import (
	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/txn"
)

// stageResiliencePreference keeps the persisted user intent consistent with
// the privileged systemd operation. The transaction restores the prior intent
// if the privileged installation/removal fails.
func stageResiliencePreference(t *txn.Transaction, p paths.Paths, enabled bool, change func() error) error {
	if err := t.BackupFile(p.CustomizeFile); err != nil {
		return err
	}
	cfg := config.Load(p)
	cfg.EnableResilience = enabled
	if err := config.Save(p, cfg); err != nil {
		return err
	}
	return change()
}

func applyResiliencePreference(p paths.Paths, enabled bool, change func() error) error {
	return txn.Run(i18n.T("网络自愈设置"), func(t *txn.Transaction) error {
		return stageResiliencePreference(t, p, enabled, change)
	})
}
