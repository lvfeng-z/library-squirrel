package pluginTaskUrlListener

import (
	"github.com/library-squirrel/backend/plugin/participation"
	"github.com/library-squirrel/backend/plugin/settingresolver"

	dto "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// ParticipationSource 参与度真相层的订阅面（真相层 participation.Manager 结构性实现）
type ParticipationSource interface {
	SubscribeChanges(handler participation.ChangeHandler) (unsubscribe func())
}

// Service 插件任务URL监听器服务（清单派生索引的查询与维护面）
type Service struct {
	manager *Manager
	// detachParticipation 参与度订阅退订函数（装配期接线一次，无并发写）
	detachParticipation func()
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

// RegisterDeclared 按清单声明的作品拉取条目登记派生索引（激活期调用）
func (s *Service) RegisterDeclared(plugin *domain.Plugin, handlers []dto.WorkFetchDeclaration) {
	s.manager.RegisterDeclared(plugin, handlers)
}

// Unregister 取消注册插件的监听器（extensionId 空则清该插件全部，非空则只清该 extensionId）
func (s *Service) Unregister(pluginPublicId string, extensionId string) {
	s.manager.Unregister(pluginPublicId, extensionId)
}

// AttachParticipation 订阅参与度真相层变更并联动派生索引：URL 监听条目在参与度词汇中无
// 独立 point，按其声明来源面 workFetch 的条目级变化联动（映射理由见
// Manager.ApplyWorkFetchEntryParticipation）。须在插件激活开始前接线——激活末位的首次
// 求值即可能发出停用变更。重复接线时先退订旧订阅；source 为 nil 等效退订
func (s *Service) AttachParticipation(source ParticipationSource) {
	if s.detachParticipation != nil {
		s.detachParticipation()
		s.detachParticipation = nil
	}
	if source == nil {
		return
	}
	s.detachParticipation = source.SubscribeChanges(func(change participation.Change) {
		if change.Point != settingresolver.PointWorkFetch {
			return
		}
		s.manager.ApplyWorkFetchEntryParticipation(change.PluginPublicId, change.ID, change.Active)
	})
}
