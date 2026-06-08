package persistentStore

import (
	"fmt"
	"path/filepath"
	"strings"
)

// StoreDir 存储子目录定义
// Path 为相对于 {workDir} 的路径
// 多级子目录通过 "/" 分隔声明，如 "store/avatar/local"
type StoreDir struct {
	Path        string
	Description string
}

// 已注册的存储子目录
var registeredDirs = []StoreDir{
	{Path: "store/resource", Description: "作品资源文件（迁移过渡用）"},
	{Path: "store/thumbnail", Description: "视频缩略图"},
	{Path: "store/avatar/local", Description: "本地作者头像"},
	{Path: "store/avatar/site", Description: "站点作者头像"},
}

// validatePath 校验路径是否以已注册子目录开头
// relPath: 如 "store/resource/author/video.mp4"、"store/avatar/local/123.jpg"
// 内部统一转换为正斜杠比较，兼容 Windows 下 filepath.Join 产出的反斜杠路径
func validatePath(relPath string) error {
	normalized := filepath.ToSlash(relPath)
	for _, dir := range registeredDirs {
		if normalized == dir.Path || strings.HasPrefix(normalized, dir.Path+"/") {
			return nil
		}
	}
	return fmt.Errorf("路径 %q 未匹配任何已注册子目录", relPath)
}
