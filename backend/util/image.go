package util

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// imageExts 支持解码的图片扩展名（与前端 WorkCard IMAGE_EXTENSIONS 对齐）
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// IsImageExt 判断扩展名是否为可解码的图片格式
func IsImageExt(ext string) bool {
	return imageExts[strings.ToLower(ext)]
}

// DecodeImageDimensions 读取图片头部，返回宽高（像素）。仅读头部几 KB，不解码全图。
// 格式不支持或文件损坏时返回 error，由调用方容错处理（宽高留 0）。
func DecodeImageDimensions(filePath string) (width, height int, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
