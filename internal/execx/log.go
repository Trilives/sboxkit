package execx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
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
	logMode     os.FileMode
)

// LogPath 默认日志文件路径（state 传 paths.Paths.State，此处不直接依赖 paths
// 包，只接收字符串，避免给 execx 添加新的包内依赖）。
func LogPath(state string) string { return filepath.Join(state, "sboxkit.log") }

// openLogFile 追加打开日志（可读，裁剪时从同一句柄读取）并设为 mode；不跟随符号链接：
// root 写日志时，被换成链接的日志文件不能把写入引向别处。
func openLogFile(path string, mode os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return nil, err
	}
	// 经句柄设定权限：不受 umask 影响，也覆盖旧版本留下的宽松权限。
	if st, err := f.Stat(); err != nil || st.Mode().Perm() != mode {
		if err := f.Chmod(mode); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// EnableLog 开启日志：Info/Ok/Warn/Error/Header 的输出额外追加写入 path
// （去除 ANSI 颜色码、带时间戳）。maxBytes<=0 时用 DefaultLogMaxBytes。日志可能含
// 订阅地址等凭据，mode 是新建与裁剪后日志的权限（paths.Paths.SecretFileMode）。
func EnableLog(path string, maxBytes int64, mode os.FileMode) error {
	if maxBytes <= 0 {
		maxBytes = DefaultLogMaxBytes
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := openLogFile(path, mode)
	if err != nil {
		return err
	}
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Close()
	}
	logFile, logPath, logMaxBytes, logMode = f, path, maxBytes, mode
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
	// 从已打开的句柄读，而不是按路径重新打开。
	data := make([]byte, st.Size())
	n, err := logFile.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return
	}
	data = data[:n]
	if int64(len(data)) <= logMaxBytes {
		return
	}
	cut := int64(len(data)) - logMaxBytes
	if idx := bytes.IndexByte(data[cut:], '\n'); idx >= 0 {
		cut += int64(idx) + 1
	}
	if err := replaceLog(data[cut:]); err != nil {
		return
	}
	logFile.Close()
	if f, err := openLogFile(logPath, logMode); err == nil {
		logFile = f
	}
}

// replaceLog 以同目录临时文件 + 改名替换日志内容，不经路径写入现有文件。
func replaceLog(tail []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(logPath), ".sboxkit.log.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(tail); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(logMode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), logPath)
}
