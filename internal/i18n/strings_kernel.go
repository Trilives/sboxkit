package i18n

func init() {
	register(map[string]string{
		"使用缓存: ":          "Using cache: ",
		"丢弃无效缓存: ":        "Discarding invalid cache: ",
		"下载: ":            "Downloading: ",
		"下载文件完整性校验失败: %s": "Downloaded file integrity check failed: %s",
		"下载文件为空: %s":      "Downloaded file is empty: %s",

		"查找最新 sing-box 版本…":            "Looking up the latest sing-box version…",
		"未找到架构 %s 的 Linux sing-box 资源": "No Linux sing-box asset found for architecture %s",
		"内核已部署: ":                      "Core deployed: ",
		"压缩包内未找到 sing-box 二进制: %s":     "sing-box binary not found inside archive: %s",

		"geo 数据已更新": "geo data updated",

		"非法压缩条目路径: %s": "Illegal archive entry path: %s",

		"已从系统包接管 %d 个种子文件（离线可用；后续可在线更新）。": "Took over %d seed file(s) from the system package (usable offline; can be updated online later).",

		"%s 完整性校验失败：期望 %s，实际 %s":                  "%s failed the integrity check: expected %s, got %s",
		"无法从 GitHub API 取得 %s 的摘要，拒绝经镜像下载未经校验的文件": "Cannot get the digest of %s from the GitHub API; refusing to download an unverified file through the mirror",
		"查询 %s 的摘要失败：%v":                          "Failed to look up the digest of %s: %v",
	})
}
