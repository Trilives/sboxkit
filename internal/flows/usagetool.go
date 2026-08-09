package flows

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/tui"
)

func UsageTool() error {
	clearToolScreen()
	fmt.Println(i18n.T(`sboxkit 使用说明

• 运行时管理：切换或固定节点、管理服务、自愈与定时器。
• 配置变更：管理订阅，修改 TUN、端口、面板、下载与分流设置。
• 工具：网络测试、日志、文件位置、连接信息和全部更新入口。
• 暂停 / 启动：主服务及 watchdog、更新定时器会一起切换。

键位：↑/↓ 移动，Space 勾选，Enter 确认，Esc 保存返回，Ctrl-R 回退。
详细命令参考：docs/COMMANDS.md 或 sboxkit --help`))
	fmt.Println()
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}
