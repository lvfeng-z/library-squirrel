package settings

// Settings 应用设置
type Settings struct {
	Initialized    bool           `json:"initialized" koanf:"initialized"`
	ProgramVersion string         `json:"programVersion" koanf:"programVersion"`
	WorkDir        string         `json:"workdir" koanf:"workdir"`
	WorkSettings   WorkSettings   `json:"workSettings" koanf:"workSettings"`
	ImportSettings ImportSettings `json:"importSettings" koanf:"importSettings"`
	PluginSettings PluginSettings `json:"pluginSettings" koanf:"pluginSettings"`
	Tour           TourSettings   `json:"tour" koanf:"tour"`
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

// TourSettings 新手引导设置
type TourSettings struct {
	FirstTimeTourPassed bool `json:"firstTimeTourPassed" koanf:"firstTimeTourPassed"`
	WorkdirTour         bool `json:"workdirTour" koanf:"workdirTour"`
	TaskTour            bool `json:"taskTour" koanf:"taskTour"`
}

// NewSettings 创建默认设置
func NewSettings() *Settings {
	return &Settings{
		Initialized:    false,
		ProgramVersion: "",
		WorkDir:        "",
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
			FirstTimeTourPassed: false,
			WorkdirTour:         false,
			TaskTour:            false,
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
