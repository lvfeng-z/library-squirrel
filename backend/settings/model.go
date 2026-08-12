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
	MergeSettings      MergeSettings      `json:"mergeSettings" koanf:"mergeSettings"`
	FsmonitorSettings  FsmonitorSettings  `json:"fsmonitor" koanf:"fsmonitor"`
}

// WorkSettings 作品相关设置
type WorkSettings struct {
	FileNameFormat string `json:"fileNameFormat" koanf:"fileNameFormat"`
}

// DefaultFileNameFormat 默认文件名模板(用户未配置或清空时回退)。
// D2 方案 B:GetFileNameFormat 空时回退此值,根治模板空→落盘名退化为 task.ext→StoreStream 删旧建新无声覆盖
const DefaultFileNameFormat = "[${author}]_[${siteWorkId}]_${siteWorkName}"

// ImportSettings 导入相关设置
type ImportSettings struct {
	MaxParallelImport        int  `json:"maxParallelImport" koanf:"maxParallelImport"`
	UpdateWorkInfoWhenImport bool `json:"updateWorkInfoWhenImport" koanf:"updateWorkInfoWhenImport"`
}

// PluginSettings 插件相关设置
type PluginSettings struct {
	AllowUnsafeEval bool `json:"allowUnsafeEval" koanf:"allowUnsafeEval"`
	RestrictedMode  bool `json:"restrictedMode" koanf:"restrictedMode"` // 受限模式：开启后启动时仅激活 bundled 插件，跳过所有第三方（出问题时安全启动的救生圈）
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

// MergeSettings 合并相关设置
type MergeSettings struct {
	Strategy string `json:"strategy" koanf:"strategy"` // 合并产物挂载策略：keep=新建 merged 保留原轨道 / overwrite=新建 merged 删原轨道
}

// FsmonitorSettings 工作目录监控设置
type FsmonitorSettings struct {
	UsnEnabled bool `json:"usnEnabled" koanf:"usnEnabled"` // USN 离线精确追溯开关（仅 Windows，需管理员运行）；默认关，离线走全量对账。详见 doc/plan/USN离线追溯方案.md D8
}

// 合并策略取值
const (
	MergeStrategyKeep      = "keep"      // 新建 merged store，保留原 videoTrack/audioTrack
	MergeStrategyOverwrite = "overwrite" // 新建 merged store，删除原轨道 store 及文件
)

// NewSettings 创建默认设置
func NewSettings() *Settings {
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
			RestrictedMode:  false,
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
		FsmonitorSettings: FsmonitorSettings{
			UsnEnabled: false,
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
