package slot

import (
	"github.com/library-squirrel/wails/internal/plugin/extension"
	domain "github.com/library-squirrel/wails/pkg"
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
