package pluginTaskUrlListener

import (
	dto "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// Service 插件任务URL监听器服务（清单派生索引的查询与维护面）
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
func (s *Service) ListListener(url string) []*PluginWithExtension {
	return s.manager.ListListener(url)
}

// RegisterDeclared 按清单声明的任务处理器条目登记派生索引（激活期调用）
func (s *Service) RegisterDeclared(plugin *domain.Plugin, handlers []dto.TaskHandlerDeclaration) {
	s.manager.RegisterDeclared(plugin, handlers)
}

// Unregister 取消注册插件的监听器（extensionId 空则清该插件全部，非空则只清该 extensionId）
func (s *Service) Unregister(pluginPublicId string, extensionId string) {
	s.manager.Unregister(pluginPublicId, extensionId)
}
