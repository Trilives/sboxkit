// 定制层字段分组交互式编辑（对应 customize.py 的 edit 部分），直接挂在
// 「配置变更」主菜单下，退出即保存本组已做的修改（外层会话负责整体回退）。
package flows

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/firewall"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

// EditFieldGroup 交互式编辑 customize.json 里的一个字段分组（部署设置 /
// 分流增强，见 config.DeploymentFields / config.OverlayFields）。直接挂在
// 「配置变更」主菜单下（与订阅管理平级），不再经过多余的「编辑定制层」
// 中间层——外层配置变更会话已对 customize.json 做了文件快照，^R 会整体回退，
// 这里退出（esc/^R 皆可）就保存本组内已做的修改即可，不需要再单独缓冲一层。
// 返回是否实际保存了改动。
func EditFieldGroup(p paths.Paths, title string, fields []string) (bool, error) {
	original := config.Load(p)
	cfg := original
	changed := false
	idx := 0
	for {
		labels := make([]string, len(fields))
		for i, k := range fields {
			labels[i] = config.FieldLabel(cfg, k)
		}
		i, err := tui.Select(i18n.T(title), labels, tui.SelectOpts{BackLabel: i18n.T("返回上层"), Initial: idx})
		if err != nil {
			break
		}
		idx = i
		key := fields[i]
		switch {
		case config.ListFields[key] != "":
			changed = editList(&cfg, key, config.ListFields[key]) || changed
		case config.BoolFields[key] != "":
			config.ToggleBool(&cfg, key)
			changed = true
		default:
			changed = editScalar(&cfg, key, config.ScalarFields[key]) || changed
		}
	}
	if !changed {
		return false, nil
	}
	if err := config.Save(p, cfg); err != nil {
		return false, err
	}
	execx.Ok(i18n.T("定制层已保存。"))
	if ferr := syncLanProxyFirewall(original, cfg); ferr != nil {
		return true, ferr
	}
	syncLogging(p, cfg)
	return true, nil
}

// syncLanProxyFirewall lan_proxy 开关变化时，按需更新防火墙放行 7890 端口。
func syncLanProxyFirewall(original, cfg config.Config) error {
	if original.LanProxy == cfg.LanProxy {
		return nil
	}
	after := cfg.LanProxy
	prompt := i18n.T("已开启局域网代理，更新防火墙放行 7890 端口？")
	if !after {
		prompt = i18n.T("已关闭局域网代理，撤销防火墙放行 7890 端口？")
	}
	ok, err := tui.Confirm(prompt, true)
	if err != nil || !ok {
		return nil
	}
	if after {
		firewall.Allow(firewall.ProxyPort)
	} else {
		firewall.Revoke(firewall.ProxyPort)
	}
	return nil
}

// syncLogging enable_file_log 开关变化时立即生效，不必等下次启动 sboxkit。
func syncLogging(p paths.Paths, cfg config.Config) {
	if !cfg.EnableFileLog {
		execx.DisableLog()
		return
	}
	if err := execx.EnableLog(execx.LogPath(p.State), int64(cfg.LogMaxMB)*1024*1024); err != nil {
		execx.Warn(i18n.T("日志启用失败：") + err.Error())
	}
}

func editList(cfg *config.Config, key, label string) bool {
	isInt := key == "tun_exclude_uids"
	changed := false
	act := 0
	for {
		items := config.StrList(*cfg, key)
		if isInt {
			items = intListToStrList(config.IntList(*cfg, key))
		}
		summary := ""
		if len(items) > 0 {
			summary = "：" + strings.Join(items, ", ")
		}
		execx.Info(fmt.Sprintf(i18n.T("%s：当前 %d 条%s"), i18n.T(label), len(items), summary))
		a, err := tui.Select(i18n.T("编辑 · ")+i18n.T(label),
			[]string{i18n.T("添加一条"), i18n.T("删除一条"), i18n.T("批量粘贴替换（逗号/空格分隔）"), i18n.T("恢复默认"), i18n.T("清空")},
			tui.SelectOpts{Initial: act})
		if err != nil {
			return changed
		}
		act = a

		ok := true
		switch a {
		case 0:
			val, err := tui.Ask(i18n.T("新增值"), tui.AskOpts{AllowEmpty: false})
			if err != nil || (isInt && !isIntStr(val)) {
				ok = false
				break
			}
			items = append(items, val)
		case 1:
			if len(items) == 0 {
				continue
			}
			di, err := tui.Select(i18n.T("删除哪一条"), items, tui.SelectOpts{})
			if err != nil {
				ok = false
				break
			}
			items = append(items[:di], items[di+1:]...)
		case 2:
			raw, err := tui.Ask(i18n.T("粘贴（逗号或空格分隔）"), tui.AskOpts{AllowEmpty: true})
			if err != nil {
				ok = false
				break
			}
			toks := config.SplitList(raw)
			if isInt {
				for _, t := range toks {
					if !isIntStr(t) {
						ok = false
						break
					}
				}
			}
			if ok {
				items = toks
			}
		case 3:
			if isInt {
				items = intListToStrList(config.IntList(config.Defaults(), key))
			} else {
				items = config.StrList(config.Defaults(), key)
			}
		case 4:
			items = []string{}
		}
		if !ok {
			execx.Warn(i18n.T("输入无效，已跳过。"))
			continue
		}
		if isInt {
			ints := make([]int, 0, len(items))
			for _, s := range items {
				n, _ := strconv.Atoi(s)
				ints = append(ints, n)
			}
			config.SetIntList(cfg, key, ints)
		} else {
			config.SetStrList(cfg, key, items)
		}
		changed = true
	}
}

func intListToStrList(items []int) []string {
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = strconv.Itoa(v)
	}
	return out
}

func isIntStr(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func editScalar(cfg *config.Config, key, label string) bool {
	cur := config.Str(*cfg, key)
	display := ""
	if config.SensitiveFields[key] && cur != "" {
		display = config.MaskSecret(cur)
	}
	val, err := tui.Ask(i18n.T(label)+i18n.T("（留空清除）"), tui.AskOpts{Default: cur, DisplayDefault: display, AllowEmpty: true})
	if err != nil {
		return false
	}
	if err := config.SetField(cfg, key, val); err != nil {
		execx.Warn(err.Error())
		return false
	}
	return true
}
