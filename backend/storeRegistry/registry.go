package storeRegistry

import (
	"fmt"
	"path/filepath"
	"strings"
)

// StoreDir 存储子目录定义。
// Path 为相对于 {workDir} 的路径；多级子目录通过 "/" 分隔声明，如 "store/avatar/local"。
type StoreDir struct {
	Path        string
	Description string
}

// RegisteredDirs 已注册的存储子目录（白名单单一源）。
// persistentStore 落盘前路径校验、fsmonitor 对账扫描与 USN 路径过滤都引用本清单。
var RegisteredDirs = []StoreDir{
	{Path: "store/resource", Description: "作品资源文件（迁移过渡用）"},
	{Path: "store/thumbnail", Description: "视频缩略图"},
	{Path: "store/avatar/local", Description: "本地作者头像"},
	{Path: "store/avatar/site", Description: "站点作者头像"},
}

// RegisteredPaths 从 RegisteredDirs 派生的路径列表（单一源派生，供遍历消费）。
var RegisteredPaths = func() []string {
	paths := make([]string, len(RegisteredDirs))
	for i, d := range RegisteredDirs {
		paths[i] = d.Path
	}
	return paths
}()

// ValidatePath 校验路径是否以已注册子目录开头（落盘前路径校验）。
// relPath 如 "store/resource/author/video.mp4"、"store/avatar/local/123.jpg"。
// 内部统一转正斜杠比较，兼容 Windows 下 filepath.Join 产出的反斜杠路径。
func ValidatePath(relPath string) error {
	normalized := filepath.ToSlash(relPath)
	for _, dir := range RegisteredDirs {
		if normalized == dir.Path || strings.HasPrefix(normalized, dir.Path+"/") {
			return nil
		}
	}
	return fmt.Errorf("路径 %q 未匹配任何已注册子目录", relPath)
}

// InScanDirs 判断 workDir 相对路径是否命中任一白名单子树（含子树根自身）。
// 离线对账扫描与 USN 路径过滤共用：变更路径须命中白名单才纳入，
// store/ 外的变更（backup/、.git/ 等）为噪声丢弃。rel 用正斜杠基准（与 file_path 一致）。
func InScanDirs(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	for _, d := range RegisteredDirs {
		if rel == d.Path || strings.HasPrefix(rel, d.Path+"/") {
			return true
		}
	}
	return false
}
