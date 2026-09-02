package appLauncher

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/library-squirrel/backend/settings"
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

// WorkDirProvider 工作目录提供者接口
type WorkDirProvider interface {
	GetWorkDir() string
}

// Service 应用启动服务
type Service struct {
	workDirProvider WorkDirProvider
}

// NewService 创建应用启动服务
func NewService(workDirProvider WorkDirProvider) *Service {
	return &Service{
		workDirProvider: workDirProvider,
	}
}

// OpenImage 使用系统默认应用打开图片资源
// filePath 为相对于资源库根目录（workdir）的相对路径
func (s *Service) OpenImage(filePath string) error {
	return s.OpenPath(filePath)
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
// filePath 为相对于资源库根目录（workdir）的相对路径
func (s *Service) OpenPath(filePath string) error {
	if err := settings.RefuseIfUnconfigured(s.workDirProvider.GetWorkDir(), "appLauncher"); err != nil {
		return err
	}
	if filePath == "" {
		return ErrInvalidPath
	}
	fullPath := filepath.Join(s.workDirProvider.GetWorkDir(), filePath)
	return openWithSystemDefault(fullPath)
}

// OpenAbsolutePath 使用系统默认应用打开绝对路径（文件或目录），不做 workdir 拼接。
// 用于导出产物等绝对路径已知的场景（Windows: cmd /c start / macOS: open / Linux: xdg-open）。
func (s *Service) OpenAbsolutePath(path string) error {
	if path == "" {
		return ErrInvalidPath
	}
	return openWithSystemDefault(path)
}

// openWithSystemDefault 按平台调用系统默认应用打开指定路径（文件或目录均适用）。
func openWithSystemDefault(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
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
