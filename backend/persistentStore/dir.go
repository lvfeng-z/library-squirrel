package persistentStore

import (
	"fmt"
	"strings"
)

// StoreDir 存储子目录定义
// Path 为相对于 {workDir}/store/ 的路径
// 多级子目录通过 "/" 分隔声明，如 "avatar/local"
type StoreDir struct {
	Path        string
	Description string
}

// 已注册的存储子目录
var registeredDirs = []StoreDir{
	{Path: "resource", Description: "作品资源文件（迁移过渡用）"},
	{Path: "thumbnail", Description: "视频缩略图"},
	{Path: "avatar/local", Description: "本地作者头像"},
	{Path: "avatar/site", Description: "站点作者头像"},
}

// validatePath 校验路径是否以已注册子目录开头
// relPath: 如 "resource/author/video.mp4"、"avatar/local/123.jpg"
func validatePath(relPath string) error {
	for _, dir := range registeredDirs {
		if relPath == dir.Path || strings.HasPrefix(relPath, dir.Path+"/") {
			return nil
		}
	}
	return fmt.Errorf("路径 %q 未匹配任何已注册子目录", relPath)
}
