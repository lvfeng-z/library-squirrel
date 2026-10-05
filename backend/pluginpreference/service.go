package pluginpreference

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

var (
	// ErrInvalidPluginID 插件 DB id 无效（非正数）
	ErrInvalidPluginID = errors.New("插件 id 无效")
	// ErrEmptyPrefKey 偏好键为空（键是插件自定义的偏好身份，不可为空）
	ErrEmptyPrefKey = errors.New("偏好键不能为空")
	// ErrNilPreferenceValue 偏好值信封缺失（整值覆写的写入单位是完整信封）
	ErrNilPreferenceValue = errors.New("偏好值不能为空")
)

// RuntimeRepository 运行时面仓储（插件读写列三操作所需；无删除——删除仅管理面）
type RuntimeRepository interface {
	// GetByKey 按 (plugin_id, pref_key) 查行，未命中返回 (nil, nil)
	GetByKey(ctx context.Context, pluginID int64, prefKey string) (*entity.PluginPreference, error)
	// UpsertByKey 按 (plugin_id, pref_key) 幂等写入：不存在建行，存在整值覆写并刷 update_time
	UpsertByKey(ctx context.Context, pluginID int64, prefKey, valueJSON string, now int64) error
	// ListByPlugin 某插件全部偏好行（pref_key 升序稳定输出）
	ListByPlugin(ctx context.Context, pluginID int64) ([]*entity.PluginPreference, error)
}

// Service 插件偏好运行时服务（插件侧读写面）：读（Get）/ 整值覆写（Set）/ 列键
//（ListKeys）三能力。不设删除方法——「忘掉」是用户权利，删除仅经管理面
// ManagementService，插件只能覆写不能销毁记忆
type Service struct {
	repo RuntimeRepository
}

// NewService 创建插件偏好运行时服务
func NewService(repo RuntimeRepository) *Service {
	return &Service{repo: repo}
}

// Get 按插件与键读取偏好值。无记录返回 (nil, nil) 不报错——无记录是合法状态
//（用户已删除或从未写入），调用方据此回落重新发起问答
func (s *Service) Get(ctx context.Context, pluginID int64, prefKey string) (*PreferenceValue, error) {
	row, err := s.repo.GetByKey(ctx, pluginID, prefKey)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return parseValue(row.Value)
}

// Set 整值覆写：同 (plugin_id, pref_key) 已存在则整值重写并刷 update_time，不存在则
// 建行。信封结构由本方法序列化保证 value 列恒为合法 JSON
func (s *Service) Set(ctx context.Context, pluginID int64, prefKey string, value *PreferenceValue) error {
	if pluginID <= 0 {
		return ErrInvalidPluginID
	}
	key := strings.TrimSpace(prefKey)
	if key == "" {
		return ErrEmptyPrefKey
	}
	if value == nil {
		return ErrNilPreferenceValue
	}
	raw, err := marshalValue(value)
	if err != nil {
		return fmt.Errorf("偏好值序列化失败: %w", err)
	}
	if err := s.repo.UpsertByKey(ctx, pluginID, key, raw, util.GetCurrentTimestamp()); err != nil {
		return fmt.Errorf("写入插件偏好失败: %w", err)
	}
	return nil
}

// ListKeys 列出该插件全部偏好键（插件自身域；跨插件键互不可见）
func (s *Service) ListKeys(ctx context.Context, pluginID int64) ([]string, error) {
	rows, err := s.repo.ListByPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.PrefKey)
	}
	return keys, nil
}
