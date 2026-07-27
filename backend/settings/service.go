package settings

import (
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/library-squirrel/backend/base/logger"
)

// Service 设置服务
type Service struct {
	k        *koanf.Koanf
	filePath string
	mu       sync.RWMutex
}

// defaultSettings 返回默认配置
func defaultSettings() *Settings {
	return &Settings{
		Initialized: false,
		WorkDir:     "",
		WorkSettings: WorkSettings{
			FileNameFormat: DefaultFileNameFormat,
		},
		ImportSettings: ImportSettings{
			MaxParallelImport:        3,
			UpdateWorkInfoWhenImport: true,
		},
		PluginSettings: PluginSettings{
			AllowUnsafeEval: false,
		},
		Tour: TourSettings{
			Completed: map[string]bool{},
		},
		RecycleBinSettings: RecycleBinSettings{
			AutoCleanupEnabled: true,
			RetentionDays:      30,
		},
		Appearance: AppearanceSettings{
			Theme: "default-light",
		},
		MergeSettings: MergeSettings{
			Strategy: MergeStrategyKeep,
		},
	}
}

// NewService 创建设置服务
func NewService(settingsFilePath string) *Service {
	k := koanf.New(".")

	// 1. 加载默认值
	k.Load(structs.Provider(defaultSettings(), "koanf"), nil)

	// 2. 加载文件配置（如果存在）
	if _, err := os.Stat(settingsFilePath); err == nil {
		if err := k.Load(file.Provider(settingsFilePath), json.Parser()); err != nil {
			logger.Log.Errorf("加载设置文件失败: %v", err)
		}
	}

	// 3. 加载环境变量（可选，当前禁用以避免意外覆盖）
	// APP_IMPORT_MAX_PARALLEL_IMPORT -> importSettings.maxParallelImport
	// k.Load(env.Provider("APP_", ".", func(s string) string {
	//     return s
	// }), nil)

	return &Service{
		k:        k,
		filePath: settingsFilePath,
	}
}

// GetSettings 获取所有设置
func (s *Service) GetSettings() *Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var settings Settings
	if err := s.k.Unmarshal("", &settings); err != nil {
		logger.Log.Errorf("获取设置失败: %v", err)
		return defaultSettings()
	}
	return &settings
}

// GetWorkDir 获取工作目录（实现 taskManager.WorkDirProvider 接口）
func (s *Service) GetWorkDir() string {
	return s.GetSettings().WorkDir
}

// GetFileNameFormat 获取文件名格式模板（实现 taskManager.FileNameFormatProvider 接口）。
// 空时回退默认模板(D2 方案 B):根治模板空→落盘名退化为 task.ext→StoreStream 删旧建新无声覆盖
func (s *Service) GetFileNameFormat() string {
	if tpl := s.GetSettings().WorkSettings.FileNameFormat; tpl != "" {
		return tpl
	}
	return DefaultFileNameFormat
}

// GetRecycleBinSettings 获取回收站自动清理设置（实现 recycleBin.RecycleBinSettingsProvider 接口）
func (s *Service) GetRecycleBinSettings() (bool, int) {
	r := s.GetSettings().RecycleBinSettings
	return r.AutoCleanupEnabled, r.RetentionDays
}

// GetMergeStrategy 获取合并产物挂载策略（实现 resource.MergeSettingsReader 接口）
func (s *Service) GetMergeStrategy() string {
	return s.GetSettings().MergeSettings.Strategy
}

// SaveSettings 保存设置变更
// changes 是设置变更列表，每项包含 path（如 "importSettings.maxParallelImport"）和 value
func (s *Service) SaveSettings(changes []SettingChange) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 将变更合并到 koanf
	for _, change := range changes {
		s.k.Set(change.Path, change.Value)
	}

	// 写回文件
	var settings Settings
	if err := s.k.Unmarshal("", &settings); err != nil {
		return err
	}

	data, err := stdjson.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// ResetSettings 重置设置到默认值
func (s *Service) ResetSettings() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.k = koanf.New(".")
	s.k.Load(structs.Provider(defaultSettings(), "koanf"), nil)

	var settings Settings
	if err := s.k.Unmarshal("", &settings); err != nil {
		return err
	}

	data, err := stdjson.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}
