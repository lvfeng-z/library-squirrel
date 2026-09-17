package tagNamespace

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// Repository 标签 namespace 维度清单仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, rows []*entity.TagNamespace) error
	// ListByValues 按维度值列表批量查询
	ListByValues(ctx context.Context, values []string) ([]*entity.TagNamespace, error)
	// ListAllOrdered 全量清单行（value 升序）
	ListAllOrdered(ctx context.Context) ([]*entity.TagNamespace, error)
	// TouchBatch 刷新既有清单行的使用记录（origin 取 MAX(现值, 来源)、last_use 刷新、label 不改写）
	TouchBatch(ctx context.Context, ids []int64, origin int64, lastUse int64) error
}

// Service 标签 namespace 维度清单服务
type Service struct {
	repo Repository
}

// NewService 创建标签 namespace 维度清单服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SyncBuiltins 内置集投影同步：内置常量集中本库缺失的 value 按权威值（value+label+origin=builtin）
// 新建清单行，既有行一律不动（insert-only）——幂等，且用户/插件写入的行与被改过的 label 不被投影
// 重置。调用点为应用启动装配期，失败即启动失败（fail-fast）；insert-only 幂等保证重启重试安全
func (s *Service) SyncBuiltins(ctx context.Context) error {
	if len(constant.BuiltinTagNamespaces) == 0 {
		return nil
	}
	values := make([]string, 0, len(constant.BuiltinTagNamespaces))
	for _, item := range constant.BuiltinTagNamespaces {
		values = append(values, item.Value)
	}
	existing, err := s.repo.ListByValues(ctx, values)
	if err != nil {
		return fmt.Errorf("内置集投影查询既有清单行失败: %w", err)
	}
	existingValues := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		existingValues[row.Value] = struct{}{}
	}
	creates := make([]*entity.TagNamespace, 0)
	for _, item := range constant.BuiltinTagNamespaces {
		if _, ok := existingValues[item.Value]; ok {
			continue
		}
		e := entity.NewTagNamespace()
		e.Value = item.Value
		e.Label = item.Label
		e.Origin = constant.ORIGIN_BUILTIN
		creates = append(creates, e)
	}
	if err := s.repo.CreateBatch(ctx, creates); err != nil {
		return fmt.Errorf("内置集投影新建清单行失败: %w", err)
	}
	return nil
}

// ListByValues 按维度值列表批量查询
func (s *Service) ListByValues(ctx context.Context, values []string) ([]*entity.TagNamespace, error) {
	return s.repo.ListByValues(ctx, values)
}

// ListAll 全量清单行（value 升序稳定输出；分组与排序归前端）
func (s *Service) ListAll(ctx context.Context) ([]*entity.TagNamespace, error) {
	return s.repo.ListAllOrdered(ctx)
}

// EnsureUsedBatch 批量登记维度值使用（关联写入链同事务调用，经 ctx 携带事务连接）：
// 值经 NormalizeDimensionValue 归一化，空串不产生清单行（无维度值=无候选）；行不存在→建
// （label 空=前端直接展示 value，origin=调用来源）；存在→origin 取 MAX(现值, 来源)（数值即
// 优先序 builtin>user>plugin，升级不降级——内置行天然不被降级）、label 永不改写、last_use 无条件刷新。
// 批内重复值折叠为单行操作
func (s *Service) EnsureUsedBatch(ctx context.Context, values []string, origin int64) error {
	distinct := make(map[string]struct{}, len(values))
	for _, v := range values {
		nv := constant.NormalizeDimensionValue(v)
		if nv == "" {
			continue
		}
		distinct[nv] = struct{}{}
	}
	if len(distinct) == 0 {
		return nil
	}
	keys := make([]string, 0, len(distinct))
	for k := range distinct {
		keys = append(keys, k)
	}
	existing, err := s.repo.ListByValues(ctx, keys)
	if err != nil {
		return fmt.Errorf("查询既有 ns 清单行失败: %w", err)
	}
	now := util.GetCurrentTimestamp()
	touchIds := make([]int64, 0, len(existing))
	for _, row := range existing {
		touchIds = append(touchIds, row.GetID())
		delete(distinct, row.Value)
	}
	if len(distinct) > 0 {
		creates := make([]*entity.TagNamespace, 0, len(distinct))
		for value := range distinct {
			e := entity.NewTagNamespace()
			e.Value = value
			e.Origin = origin
			e.LastUse = now
			creates = append(creates, e)
		}
		if err := s.repo.CreateBatch(ctx, creates); err != nil {
			return fmt.Errorf("新建 ns 清单行失败: %w", err)
		}
	}
	return s.repo.TouchBatch(ctx, touchIds, origin, now)
}
