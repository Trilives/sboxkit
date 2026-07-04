package execx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// DefaultLogMaxBytes 单个日志文件的默认体量上限；超出后动态裁剪掉最旧内容，
// 只保留尾部，避免日志无限增长占满磁盘。
const DefaultLogMaxBytes = 5 * 1024 * 1024 // 5MB

var (
	logMu       sync.Mutex
	logFile     *os.File
	logPath     string
	logMaxBytes int64
)

// LogPath 默认日志文件路径（state 传 paths.Paths.State，此处不直接依赖 paths
// 包，只接收字符串，避免给 execx 添加新的包内依赖）。
func LogPath(state string) string { return filepath.Join(state, "sboxkit.log") }

// EnableLog 开启日志：Info/Ok/Warn/Error/Header 的输出额外追加写入 path
// （去除 ANSI 颜色码、带时间戳）。maxBytes<=0 时用 DefaultLogMaxBytes。
func EnableLog(path string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Close()
	}
	logFile, logPath, logMaxBytes = f, path, maxBytes
	return nil
}

// DisableLog 关闭日志（用户在定制层里关掉「启用日志」时立即生效，不必重启）。
func DisableLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

var ansiLogRe = regexp.MustCompile(`\033\[[0-9;?]*[A-Za-z]`)

func writeLog(line string) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(logFile, "[%s] %s\n", ts, ansiLogRe.ReplaceAllString(line, ""))
	trimIfOversize()
}

// trimIfOversize 超过上限时只保留尾部内容（按行边界裁切），实现「动态删除保持
// 一定大小」。调用方需已持有 logMu。
func trimIfOversize() {
	st, err := logFile.Stat()
	if err != nil || st.Size() <= logMaxBytes {
		return
	}
	data, err := os.ReadFile(logPath)
	if err != nil || int64(len(data)) <= logMaxBytes {
		return
	}
	cut := int64(len(data)) - logMaxBytes
	if idx := bytes.IndexByte(data[cut:], '\n'); idx >= 0 {
		cut += int64(idx) + 1
	}
	tail := data[cut:]
	logFile.Close()
	os.WriteFile(logPath, tail, 0o644)
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		logFile = f
	}
}
