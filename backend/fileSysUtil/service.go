package fileSysUtil

import (
	"errors"

	"github.com/sqweek/dialog"
)

// OpenDialogResult 打开对话框结果
type OpenDialogResult struct {
	Canceled  bool     `json:"canceled"`
	FilePaths []string `json:"filePaths"`
}

// 错误定义
var (
	ErrDialogFailed = errors.New("dialog failed")
)

// Service 文件系统工具服务
type Service struct {
	defaultPath string
}

// NewService 创建服务
func NewService(defaultPath string) *Service {
	return &Service{
		defaultPath: defaultPath,
	}
}

// SetDefaultPath 设置默认路径
func (s *Service) SetDefaultPath(defaultPath string) {
	s.defaultPath = defaultPath
}

// DirSelect 打开目录/文件选择对话框
// openFile: true=选择文件, false=选择目录
// isModal: 是否模态对话框（当前实现忽略此参数）
func (s *Service) DirSelect(openFile bool, isModal bool) (*OpenDialogResult, error) {
	var result OpenDialogResult

	if openFile {
		// 选择文件
		path, err := dialog.File().
			Filter("All Files", "*").
			SetStartDir(s.defaultPath).
			Load()
		if err != nil {
			if err == dialog.ErrCancelled {
				result.Canceled = true
				return &result, nil
			}
			return nil, ErrDialogFailed
		}
		result.FilePaths = []string{path}
	} else {
		// 选择目录
		path, err := dialog.Directory().
			SetStartDir(s.defaultPath).
			Browse()
		if err != nil {
			if err == dialog.ErrCancelled {
				result.Canceled = true
				return &result, nil
			}
			return nil, ErrDialogFailed
		}
		result.FilePaths = []string{path}
	}

	return &result, nil
}

// DirSelectMultiple 打开多选目录/文件选择对话框
// 注意：sqweek/dialog 库对于文件多选需要使用多个参数，这里简化处理
func (s *Service) DirSelectMultiple(openFile bool, isModal bool) (*OpenDialogResult, error) {
	var result OpenDialogResult

	// 对于多选文件场景，由于 sqweek/dialog 不直接支持多选文件，
	// 这里返回与 DirSelect 相同的结果
	path, err := dialog.Directory().
		SetStartDir(s.defaultPath).
		Browse()
	if err != nil {
		if err == dialog.ErrCancelled {
			result.Canceled = true
			return &result, nil
		}
		return nil, ErrDialogFailed
	}
	result.FilePaths = []string{path}

	return &result, nil
}
