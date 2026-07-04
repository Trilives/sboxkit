package i18n

func init() {
	register(map[string]string{
		"当前版本：%s，正在查询最新版本…":        "Current version: %s, checking for the latest…",
		"已是最新版本（%s）。":              "Already on the latest version (%s).",
		"发现新版本 %s（当前 %s），现在更新？":    "New version %s available (current %s). Update now?",
		"sboxkit 已更新到 %s，下次运行即生效。": "sboxkit updated to %s; takes effect next run.",

		"发行版没有有效的版本号":                 "The release has no valid version number",
		"未找到本机架构的发行包: %s":             "No release asset found for this architecture: %s",
		"发行版缺少 checksums.txt，无法校验完整性": "The release is missing checksums.txt; cannot verify integrity",
		"下载 sboxkit: ":                "Downloading sboxkit: ",
		"SHA-256 校验通过。":               "SHA-256 verification passed.",
		"checksums.txt 里没有 %s 的记录":    "No entry for %s in checksums.txt",
		"SHA-256 校验失败：期望 %s，实际 %s":    "SHA-256 verification failed: expected %s, got %s",
		"解压后未找到 sboxkit 可执行文件":        "sboxkit executable not found after extracting",
		"新版本二进制无法正常运行，已放弃更新：%w":       "The new binary failed to run; update aborted: %w",
		"非法压缩条目路径: %s":                "Illegal archive entry path: %s",
		"无法定位当前运行的可执行文件":              "Could not locate the currently running executable",
		"已把当前运行的可执行文件迁移为托管版本 %s。":     "Migrated the currently running executable into managed version %s.",
		"sboxkit 已更新到 %s。":            "sboxkit updated to %s.",
		"新版本启动校验失败，回退到旧版本：%v":         "The new version failed its startup check; rolling back to the previous version: %v",
		"已回退到原版本：%w":                  "Rolled back to the original version: %w",
		"首次自更新需要把 ":                   "First self-update needs to take over ",
		" 接管为指向托管版本的符号链接":             " as a symlink pointing at the managed version",

		"更新 sboxkit 自身":       "Update sboxkit itself",
		"更新到稳定版":              "Update to stable",
		"更新到预览版（尝鲜，可能不稳定）":    "Update to preview (early access, may be unstable)",
		"回退到稳定版 %s":           "Roll back to stable %s",
		"已回退到稳定版 %s，下次运行即生效。": "Rolled back to stable %s; takes effect next run.",
		"仓库还没有任何发行版":          "The repository has no releases yet",
		"尚未记录过稳定版，无法回退":       "No stable version recorded yet; cannot roll back",
		"记录的稳定版二进制无法正常运行：%w":  "The recorded stable binary failed to run: %w",
		"回退后校验失败：%w":          "Post-rollback verification failed: %w",
	})
}
