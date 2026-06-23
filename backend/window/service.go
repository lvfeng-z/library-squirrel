package window

// Service 窗口服务
type Service struct {
	// hwndProvider 延迟返回主窗口句柄（构造时句柄可能尚未就绪，运行时读取）
	hwndProvider func() uintptr
}

// NewService 创建窗口服务
func NewService(hwndProvider func() uintptr) *Service {
	return &Service{hwndProvider: hwndProvider}
}

// SetTitleBarColor 设置主窗口标题栏背景色与文字色
// bg/text 为 #RRGGBB 格式
func (s *Service) SetTitleBarColor(bg, text string) error {
	return setTitleColor(s.hwndProvider(), bg, text)
}
