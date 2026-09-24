package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
)

// SettingItem 设置项（声明 + 当前值），返回给前端渲染表单
type SettingItem struct {
	Key         string              `json:"key"`
	Type        string              `json:"type"`
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Encrypted   bool                `json:"encrypted"`
	Group       string              `json:"group,omitempty"`
	Order       int                 `json:"order,omitempty"`
	Options     []dto.SettingOption `json:"options,omitempty"`
	Min         *int                `json:"min,omitempty"`
	Max         *int                `json:"max,omitempty"`
	Value       string              `json:"value"`
}

// ParticipationEvalTrigger 设置落库后的参与度求值触发器（参与度真相层 participation.Manager
// 结构性实现，app 装配注入）：触发为异步排队——落库即返回保存响应；同插件求值串行，
// 在途完成后仅跑最新输入。插件未激活（无会话）或清单未声明 settingsResolver 时为空操作
type ParticipationEvalTrigger interface {
	TriggerEvaluation(pluginPublicId string)
}

// PluginSettingService 插件设置服务（组合声明与存储）
type PluginSettingService struct {
	pluginRepo  *PluginRepository
	storage     *PluginStorageService
	rootPath    string
	evalTrigger ParticipationEvalTrigger
}

// NewPluginSettingService 创建插件设置服务。evalTrigger 为设置落库后的参与度求值触发器，
// 可为 nil（未装配时落库不触发求值——纯测试装配）
func NewPluginSettingService(pluginRepo *PluginRepository, storage *PluginStorageService, rootPath string, evalTrigger ParticipationEvalTrigger) *PluginSettingService {
	return &PluginSettingService{pluginRepo: pluginRepo, storage: storage, rootPath: rootPath, evalTrigger: evalTrigger}
}

// GetSettings 获取插件设置项（声明 + 当前值，加密项已解密）
func (s *PluginSettingService) GetSettings(ctx context.Context, publicId string) ([]SettingItem, error) {
	plugin, err := s.pluginRepo.GetByPublicId(ctx, publicId)
	if err != nil {
		return nil, fmt.Errorf("获取插件失败: %w", err)
	}
	if plugin == nil {
		return nil, fmt.Errorf("插件不存在: %s", publicId)
	}

	declarations, err := s.loadSettingDeclarations(plugin)
	if err != nil {
		return nil, err
	}

	values, err := s.storage.GetAllValues(ctx, plugin.GetID())
	if err != nil {
		return nil, fmt.Errorf("读取设置值失败: %w", err)
	}

	items := make([]SettingItem, 0, len(declarations))
	for _, d := range declarations {
		var val string
		if entry, exists := values[d.Key]; exists {
			val = entry.Value
		} else {
			val = d.Default
		}
		items = append(items, toSettingItem(d, val))
	}
	return items, nil
}

// SaveSetting 保存单个设置项（按声明 encrypted 路由加密/明文）
func (s *PluginSettingService) SaveSetting(ctx context.Context, publicId, key, value string) error {
	plugin, err := s.pluginRepo.GetByPublicId(ctx, publicId)
	if err != nil {
		return fmt.Errorf("获取插件失败: %w", err)
	}
	if plugin == nil {
		return fmt.Errorf("插件不存在: %s", publicId)
	}

	declarations, err := s.loadSettingDeclarations(plugin)
	if err != nil {
		return err
	}
	decl := findSettingDeclaration(declarations, key)
	if decl == nil {
		return fmt.Errorf("未知的设置项: %s", key)
	}

	var schemaVer int64
	if plugin.ConfigSchemaVersion.Valid {
		schemaVer = plugin.ConfigSchemaVersion.Int64
	}
	if decl.Encrypted {
		if err := s.storage.SetValueEncrypted(ctx, plugin.GetID(), key, value, schemaVer); err != nil {
			return err
		}
		s.triggerParticipationEval(publicId)
		return nil
	}
	if err := s.storage.SetValue(ctx, plugin.GetID(), key, value, schemaVer); err != nil {
		return err
	}
	s.triggerParticipationEval(publicId)
	return nil
}

// ResetSetting 重置设置项为声明默认值（删除存储值）
func (s *PluginSettingService) ResetSetting(ctx context.Context, publicId, key string) error {
	plugin, err := s.pluginRepo.GetByPublicId(ctx, publicId)
	if err != nil {
		return fmt.Errorf("获取插件失败: %w", err)
	}
	if plugin == nil {
		return fmt.Errorf("插件不存在: %s", publicId)
	}
	if err := s.storage.DeleteValue(ctx, plugin.GetID(), key); err != nil {
		return err
	}
	s.triggerParticipationEval(publicId)
	return nil
}

// triggerParticipationEval 设置落库成功后触发该插件的参与度求值（ResetSetting 不经
// SaveSetting，两落库点各自挂接；异步排队，未装配触发器时为空操作）
func (s *PluginSettingService) triggerParticipationEval(publicId string) {
	if s.evalTrigger != nil {
		s.evalTrigger.TriggerEvaluation(publicId)
	}
}

// loadSettingDeclarations 从插件 plugin.json 根级 settings 段读取用户设置项声明；
// 清单未声明该段时返回空（无设置项）
func (s *PluginSettingService) loadSettingDeclarations(plugin *entity.Plugin) ([]dto.SettingDeclaration, error) {
	manifestPath := filepath.Join(s.rootPath, plugin.RootPath.String, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("读取 plugin.json 失败: %w", err)
	}
	var manifest dto.PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析 plugin.json 失败: %w", err)
	}
	return manifest.Settings, nil
}

func findSettingDeclaration(declarations []dto.SettingDeclaration, key string) *dto.SettingDeclaration {
	for i := range declarations {
		if declarations[i].Key == key {
			return &declarations[i]
		}
	}
	return nil
}

func toSettingItem(d dto.SettingDeclaration, value string) SettingItem {
	return SettingItem{
		Key:         d.Key,
		Type:        d.Type,
		Title:       d.Title,
		Description: d.Description,
		Encrypted:   d.Encrypted,
		Group:       d.Group,
		Order:       d.Order,
		Options:     d.Options,
		Min:         d.Min,
		Max:         d.Max,
		Value:       value,
	}
}
