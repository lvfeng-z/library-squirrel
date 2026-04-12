package appLauncher

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ExternalAppEnum 外部应用枚举
type ExternalAppEnum int

const (
	ExternalAppMicrosoftPhotos   ExternalAppEnum = 1
	ExternalAppMicrosoftMoviesTv ExternalAppEnum = 2
	ExternalAppPotPlayer         ExternalAppEnum = 3
)

// 错误定义
var (
	ErrInvalidApp  = errors.New("invalid external app")
	ErrInvalidPath = errors.New("invalid file path")
	ErrOpenFailed  = errors.New("failed to open file")
)

// Service 应用启动服务
type Service struct {
	workDir string
}

// NewService 创建应用启动服务
func NewService(workDir string) *Service {
	return &Service{
		workDir: workDir,
	}
}

// SetWorkDir 设置工作目录
func (s *Service) SetWorkDir(workDir string) {
	s.workDir = workDir
}

// OpenImage 使用微软照片打开图片
func (s *Service) OpenImage(url string) error {
	if url == "" {
		return ErrInvalidPath
	}

	// 拼接完整路径
	fullPath := filepath.Join(s.workDir, url)
	return s.openWithMicrosoftPhotos(fullPath)
}

// openWithMicrosoftPhotos 使用微软照片打开文件
func (s *Service) openWithMicrosoftPhotos(filePath string) error {
	// Windows 上使用 cmd /c start 来打开文件
	// 对于微软照片，实际上使用默认的图片查看器更合适
	cmd := exec.Command("cmd", "/c", "start", "", filePath)
	return cmd.Run()
}

// Open 打开指定应用
func (s *Service) Open(app ExternalAppEnum, filePath string) error {
	if filePath == "" {
		return ErrInvalidPath
	}

	switch app {
	case ExternalAppMicrosoftPhotos:
		return s.openWithMicrosoftPhotos(filePath)
	case ExternalAppMicrosoftMoviesTv:
		return s.openWithMicrosoftMoviesTv(filePath)
	case ExternalAppPotPlayer:
		return s.openWithPotPlayer(filePath)
	default:
		return ErrInvalidApp
	}
}

// openWithMicrosoftMoviesTv 使用微软电影和电视打开文件
func (s *Service) openWithMicrosoftMoviesTv(filePath string) error {
	// Windows 上通过 cmd start 打开视频文件
	cmd := exec.Command("cmd", "/c", "start", "video:", filePath)
	return cmd.Run()
}

// openWithPotPlayer 使用 PotPlayer 打开文件
func (s *Service) openWithPotPlayer(filePath string) error {
	// PotPlayer 的路径（如果已安装）
	potPlayerPath := "C:\\Program Files\\DAUM\\PotPlayer\\PotPlayerMini64.exe"
	cmd := exec.Command(potPlayerPath, filePath)
	return cmd.Run()
}

// OpenPath 使用系统默认应用打开文件
func (s *Service) OpenPath(filePath string) error {
	if filePath == "" {
		return ErrInvalidPath
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	default:
		return ErrOpenFailed
	}
	return cmd.Run()
}

// OpenExternal 使用系统默认浏览器打开 URL
func (s *Service) OpenExternal(url string) error {
	if url == "" {
		return errors.New("invalid URL")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return ErrOpenFailed
	}
	return cmd.Run()
}
