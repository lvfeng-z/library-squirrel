package fileSysUtil

import (
	"errors"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenDialogResult 打开对话框结果
type OpenDialogResult struct {
	Canceled  bool     `json:"canceled"`
	FilePaths []string `json:"filePaths"`
}

// OpenDialogOptions 打开对话框选项
type OpenDialogOptions struct {
	Title          string                   `json:"title"`
	DefaultPath    string                   `json:"defaultPath"`
	CanChooseFiles bool                     `json:"canChooseFiles"`
	CanChooseDirs  bool                     `json:"canChooseDirs"`
	MultiSelect    bool                     `json:"multiSelect"`
	Filters        []application.FileFilter `json:"filters"`
}

// 错误定义
var (
	ErrDialogFailed  = errors.New("dialog failed")
	ErrDialogNotInit = errors.New("dialog service not initialized")
)

// Service 文件系统工具服务
type Service struct {
	defaultPath string
	app         *application.App
	window      application.Window
}

// NewService 创建服务（Wails App 延迟注入）
func NewService(defaultPath string) *Service {
	return &Service{
		defaultPath: defaultPath,
	}
}

// SetApp 设置 Wails 应用实例（延迟注入，解决初始化时序问题）
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// SetWindow 设置主窗口实例（用于模态对话框附着）
func (s *Service) SetWindow(window application.Window) {
	s.window = window
}

// SetDefaultPath 设置默认路径
func (s *Service) SetDefaultPath(defaultPath string) {
	s.defaultPath = defaultPath
}

// OpenDialog 打开文件/目录选择对话框
func (s *Service) OpenDialog(options *OpenDialogOptions) (*OpenDialogResult, error) {
	if s.app == nil {
		return nil, ErrDialogNotInit
	}

	dialog := s.app.Dialog.OpenFile()

	if s.window != nil {
		dialog.AttachToWindow(s.window)
	}

	if options.Title != "" {
		dialog.SetTitle(options.Title)
	}

	dir := options.DefaultPath
	if dir == "" {
		dir = s.defaultPath
	}
	if dir != "" {
		dialog.SetDirectory(dir)
	}

	dialog.CanChooseFiles(options.CanChooseFiles)
	dialog.CanChooseDirectories(options.CanChooseDirs)

	for _, filter := range options.Filters {
		dialog.AddFilter(filter.DisplayName, filter.Pattern)
	}

	var result OpenDialogResult

	if options.MultiSelect {
		paths, err := dialog.PromptForMultipleSelection()
		if err != nil {
			if strings.Contains(err.Error(), "cancelled") {
				result.Canceled = true
				return &result, nil
			}
			return nil, ErrDialogFailed
		}
		result.FilePaths = paths
	} else {
		path, err := dialog.PromptForSingleSelection()
		if err != nil {
			if strings.Contains(err.Error(), "cancelled") {
				result.Canceled = true
				return &result, nil
			}
			return nil, ErrDialogFailed
		}
		if path != "" {
			result.FilePaths = []string{path}
		}
	}

	return &result, nil
}
