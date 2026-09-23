package stickymemory

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// Repository 粘性记忆仓储接口（由 service 定义所需的数据库操作，提供方实现）
type Repository interface {
	// UpsertByKey 按 (domain, context_key) 幂等写入：不存在则建行，存在则重写 value 并刷 update_time
	UpsertByKey(ctx context.Context, domain, contextKey, value string, now int64) error
	// GetByKey 按 (domain, context_key) 查行，未命中返回 gorm.ErrRecordNotFound
	GetByKey(ctx context.Context, domain, contextKey string) (*entity.StickyMemory, error)
	// ListAllOrdered 全量记忆行（domain 升序、update_time 降序的稳定输出；分组归调用侧）
	ListAllOrdered(ctx context.Context) ([]*entity.StickyMemory, error)
	// Delete 按 id 删除（本实体无软删列，为物理删除）
	Delete(ctx context.Context, id int64) error
}

// Service 粘性记忆服务：交互冲突面显选的记/取/忘/列。记忆是消歧优化而非正确性依赖——
// 读取失败按未命中处理，调用方自然回落到询问用户，不因记忆层故障阻塞交互
type Service struct {
	repo Repository
}

// NewService 创建粘性记忆服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Remember 记住一次显选（upsert 幂等）：同 (domain, context_key) 重写 value 并刷
// update_time，行 id 与 create_time 保持首记时刻不变。由冲突显选且路由成功的写入点调用
func (s *Service) Remember(ctx context.Context, domain, contextKey, value string) error {
	if err := s.repo.UpsertByKey(ctx, domain, contextKey, value, util.GetCurrentTimestamp()); err != nil {
		return fmt.Errorf("写入粘性记忆失败: %w", err)
	}
	return nil
}

// Recall 按域与上下文键取回显选值。命中返回 (显选候选全键, true)；未命中或读取失败返回
// ("", false)——失败等同未命中，调用方回落询问
func (s *Service) Recall(ctx context.Context, domain, contextKey string) (string, bool) {
	row, err := s.repo.GetByKey(ctx, domain, contextKey)
	if err != nil || row == nil {
		return "", false
	}
	return row.Value, true
}

// Forget 按 id 删除一条记忆（管理入口的删除动作；删除后同冲突自然重新询问）
func (s *Service) Forget(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("删除粘性记忆失败: %w", err)
	}
	return nil
}

// List 全量记忆行（含 id/domain/context_key/value 与创建/更新时间戳；domain 分组与
// 展示排序归调用侧处理）
func (s *Service) List(ctx context.Context) ([]*entity.StickyMemory, error) {
	return s.repo.ListAllOrdered(ctx)
}
