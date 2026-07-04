package i18n

func init() {
	register(map[string]string{
		"返回": "Back",

		"↑/↓ 选择   ⏎ 确认   esc %s   ^R %s": "↑/↓ select   ⏎ confirm   esc %s   ^R %s",
		"  ▲ 上方还有 %d 项":                  "  ▲ %d more above",
		"  ▼ 下方还有 %d 项":                  "  ▼ %d more below",

		"↑/↓ 移动   空格 勾选   ⏎ 确认   esc 取消": "↑/↓ move   space toggle   ⏎ confirm   esc cancel",

		"  回车) %s    r) %s\n": "  enter) %s    r) %s\n",
		"请选择: ":               "Select: ",
		"无效选择，请重输。":           "Invalid choice, please try again.",
		"  输入编号(逗号分隔)切换勾选，回车确认，q 取消": "  Enter number(s) (comma separated) to toggle, Enter to confirm, q to cancel",
		"操作: ": "Action: ",

		"不能为空。": "Cannot be empty.",
	})
}
