package pluginpreference

import (
	"context"
	"fmt"
	"strings"
)

// ManagementRepository 管理面仓储（列表 + 删除；删除能力只在本面 reachable）
type ManagementRepository interface {
	// ListAllWithPlugin 全量偏好条目 + 归属插件显示信息（plugin_id 升序、同插件内
	// update_time 降序，管理页按插件分组的展示基准）
	ListAllWithPlugin(ctx context.Context) ([]EntryWithPlugin, error)
	// ListByPublicIdWithPlugin 按插件公开 ID 查该插件偏好条目 + 显示信息（update_time 降序）
	ListByPublicIdWithPlugin(ctx context.Context, pluginPublicId string) ([]EntryWithPlugin, error)
	// Delete 按条目 id 物理删除（本实体无软删列）
	Delete(ctx context.Context, id int64) error
}

// EntryWithPlugin 管理列表读模型：偏好条目 + 归属插件显示信息（JOIN plugin 表组装；
// 插件行不物理删，偏好行的归属显示名恒可查）。Value 为原始 JSON 文本，信封拆解归
// Handler 展示组装
type EntryWithPlugin struct {
	ID             int64
	PluginID       int64
	PrefKey        string
	Value          string
	UpdateTime     int64
	PluginPublicId string
	PluginName     string
}

// ManagementService 插件偏好管理面服务：记忆管理页与插件设置区的列表查询 + 按条目
// 删除。删除仅经本面（「忘掉」是用户权利）——删除后运行时读取即无记录，插件下次
// 问答重新发起
type ManagementService struct {
	repo ManagementRepository
}

// NewManagementService 创建插件偏好管理面服务
func NewManagementService(repo ManagementRepository) *ManagementService {
	return &ManagementService{repo: repo}
}

// ListAll 全量偏好条目（含归属插件显示信息），记忆管理页按插件分组消费
func (s *ManagementService) ListAll(ctx context.Context) ([]EntryWithPlugin, error) {
	return s.repo.ListAllWithPlugin(ctx)
}

// ListByPlugin 按插件公开 ID 列该插件偏好条目（插件设置区只读列表消费）
func (s *ManagementService) ListByPlugin(ctx context.Context, pluginPublicId string) ([]EntryWithPlugin, error) {
	publicId := strings.TrimSpace(pluginPublicId)
	if publicId == "" {
		return nil, fmt.Errorf("插件公开 ID 不能为空")
	}
	return s.repo.ListByPublicIdWithPlugin(ctx, publicId)
}

// Delete 按条目 id 删除一条偏好（管理面唯一删除入口）
func (s *ManagementService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("条目 id 无效: %d", id)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("删除插件偏好失败: %w", err)
	}
	return nil
}
