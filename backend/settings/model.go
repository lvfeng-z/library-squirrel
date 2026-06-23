package settings

// Settings 应用设置
type Settings struct {
	Initialized        bool               `json:"initialized" koanf:"initialized"`
	WorkDir            string             `json:"workdir" koanf:"workdir"`
	WorkSettings       WorkSettings       `json:"workSettings" koanf:"workSettings"`
	ImportSettings     ImportSettings     `json:"importSettings" koanf:"importSettings"`
	PluginSettings     PluginSettings     `json:"pluginSettings" koanf:"pluginSettings"`
	Tour               TourSettings       `json:"tour" koanf:"tour"`
	RecycleBinSettings RecycleBinSettings `json:"recycleBin" koanf:"recycleBin"`
	Appearance         AppearanceSettings `json:"appearance" koanf:"appearance"`
}

// WorkSettings 作品相关设置
type WorkSettings struct {
	FileNameFormat string `json:"fileNameFormat" koanf:"fileNameFormat"`
}

// ImportSettings 导入相关设置
type ImportSettings struct {
	MaxParallelImport        int  `json:"maxParallelImport" koanf:"maxParallelImport"`
	UpdateWorkInfoWhenImport bool `json:"updateWorkInfoWhenImport" koanf:"updateWorkInfoWhenImport"`
}

// PluginSettings 插件相关设置
type PluginSettings struct {
	AllowUnsafeEval bool `json:"allowUnsafeEval" koanf:"allowUnsafeEval"`
}

// TourSettings 向导完成状态，按向导 ID 记录是否已完成
type TourSettings struct {
	Completed map[string]bool `json:"completed" koanf:"completed"`
}

// RecycleBinSettings 回收站设置
type RecycleBinSettings struct {
	AutoCleanupEnabled bool `json:"autoCleanupEnabled" koanf:"autoCleanupEnabled"` // 是否启用自动清理
	RetentionDays      int  `json:"retentionDays" koanf:"retentionDays"`           // 回收站保留天数，超过后自动清理
}

// AppearanceSettings 外观设置
type AppearanceSettings struct {
	Theme string `json:"theme" koanf:"theme"` // 当前主题 id
}

// NewSettings 创建默认设置
func NewSettings() *Settings {
	return &Settings{
		Initialized: false,
		WorkDir:     "",
		WorkSettings: WorkSettings{
			FileNameFormat: "[${author}]_[${siteWorkId}]_${siteWorkName}",
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
	}
}

// GetID 实现 Entity 接口（Settings 不存储在数据库，不需要实现）
func (s *Settings) GetID() int64 {
	return 0
}

// SetID 实现 Entity 接口
func (s *Settings) SetID(id int64) {
}

// GetCreateTime 实现 Entity 接口
func (s *Settings) GetCreateTime() int64 {
	return 0
}

// SetCreateTime 实现 Entity 接口
func (s *Settings) SetCreateTime(time int64) {
}

// GetUpdateTime 实现 Entity 接口
func (s *Settings) GetUpdateTime() int64 {
	return 0
}

// SetUpdateTime 实现 Entity 接口
func (s *Settings) SetUpdateTime(time int64) {
}

// SettingChange 设置变更项
type SettingChange struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
