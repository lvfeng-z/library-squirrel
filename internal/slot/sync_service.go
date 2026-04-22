package slot

import (
	"github.com/gin-gonic/gin"
	"github.com/library-squirrel/wails/internal/plugin/extension"
	domain "github.com/library-squirrel/wails/pkg"
)

// SlotEvent 插槽事件类型
type SlotEvent string

const (
	// SlotEventRegister 插槽注册
	SlotEventRegister SlotEvent = "slot-register"
	// SlotEventUnregister 插槽注销
	SlotEventUnregister SlotEvent = "slot-unregister"
	// SlotEventSyncAll 全量同步
	SlotEventSyncAll SlotEvent = "slot-sync-all"
)

// SlotSyncMessage 插槽同步消息
type SlotSyncMessage struct {
	Event  SlotEvent   `json:"event"`
	SlotID string      `json:"slotId,omitempty"`
	Slot   interface{} `json:"slot,omitempty"`
	Slots  interface{} `json:"slots,omitempty"`
}

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

// SyncToRenderer 同步到渲染进程（通过 HTTP 响应）
func (s *SlotSyncService) SyncToRenderer(c *gin.Context) error {
	slots := s.registry.GetSlotConfigs()

	message := SlotSyncMessage{
		Event: SlotEventSyncAll,
		Slots: slots,
	}

	c.JSON(200, message)
	return nil
}
