package pluginTaskUrlListener

// Service 插件任务URL监听器服务
type Service struct {
	manager *Manager
}

// NewService 创建服务
func NewService(manager *Manager) *Service {
	return &Service{
		manager: manager,
	}
}

// ListListener 根据URL获取监听此链接的插件列表
func (s *Service) ListListener(url string) []*PluginWithContribution {
	return s.manager.ListListener(url)
}

// Register 注册插件的URL监听器
func (s *Service) Register(plugin *PluginWithContribution, patterns []string) {
	s.manager.Register(plugin, patterns)
}

// Unregister 取消注册插件的所有监听器
func (s *Service) Unregister(pluginPublicId string) {
	s.manager.Unregister(pluginPublicId)
}
