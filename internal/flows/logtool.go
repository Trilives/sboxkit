package flows

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

const logTailLines = 200

func LogTool(p paths.Paths) error {
	choice, err := tui.Select(i18n.T("查看日志"), []string{
		i18n.T("sing-box 内核日志"), i18n.T("sboxkit TUI 日志"),
	}, tui.SelectOpts{BackLabel: i18n.T("返回上层")})
	if err != nil {
		return nil
	}
	clearToolScreen()
	if choice == 0 {
		execx.Header(i18n.T("sing-box 内核日志"))
		_, err = execx.Run([]string{"journalctl", "--no-pager", "-n", fmt.Sprint(logTailLines), "-u", sysd.DefaultName + ".service"}, nil)
	} else {
		execx.Header(i18n.T("sboxkit TUI 日志"))
		err = printFileTail(execx.LogPath(p.State), logTailLines)
	}
	if err != nil {
		execx.Warn(err.Error())
	}
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}

func printFileTail(path string, limit int) error {
	lines, err := readFileTail(path, limit)
	if err != nil {
		return err
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func readFileTail(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(i18n.T("日志文件尚不存在：%s"), path)
		}
		return nil, err
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func clearToolScreen() {
	if tui.UseTUI() {
		fmt.Print("\033[2J\033[H")
	}
}
