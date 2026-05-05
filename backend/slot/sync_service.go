package slot

import (
	domain "github.com/library-squirrel/wails/backend/base"
	"github.com/library-squirrel/wails/backend/plugin/extension"
)

// SlotSyncService 插槽同步服务
type SlotSyncService struct {
	registry *extension.SlotRegistry
}

// NewSlotSyncService 创建插槽同步服务
func NewSlotSyncService(registry *extension.SlotRegistry) *SlotSyncService {
	return &SlotSyncService{
		registry: registry,
	}
}

// GetAllSlots 获取所有插槽
func (s *SlotSyncService) GetAllSlots() []*domain.SlotConfig {
	return s.registry.GetSlotConfigs()
}
