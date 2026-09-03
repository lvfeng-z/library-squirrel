package settings

// Settings 应用设置
type Settings struct {
	WorkDir            string                   `json:"workdir" koanf:"workdir"`
	WorkSettings       WorkSettings             `json:"workSettings" koanf:"workSettings"`
	ImportSettings     ImportSettings           `json:"importSettings" koanf:"importSettings"`
	PluginSettings     PluginSettings           `json:"pluginSettings" koanf:"pluginSettings"`
	Tour               TourSettings             `json:"tour" koanf:"tour"`
	RecycleBinSettings RecycleBinSettings       `json:"recycleBin" koanf:"recycleBin"`
	Appearance         AppearanceSettings       `json:"appearance" koanf:"appearance"`
	MergeSettings      MergeSettings            `json:"mergeSettings" koanf:"mergeSettings"`
	FsmonitorSettings  FsmonitorSettings        `json:"fsmonitor" koanf:"fsmonitor"`
	BackupGovernance   BackupGovernanceSettings `json:"backupGovernance" koanf:"backupGovernance"`
	ExportSettings     ExportSettings           `json:"exportSettings" koanf:"exportSettings"`
	ShareSettings      ShareSettings            `json:"shareSettings" koanf:"shareSettings"`
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

// AppearanceSettings 外观与交互偏好设置
type AppearanceSettings struct {
	Theme              string `json:"theme" koanf:"theme"`                           // 当前主题 id
	MultiSelectEnabled bool   `json:"multiSelectEnabled" koanf:"multiSelectEnabled"` // 主页多选模式开关（默认关；开启后作品/作品集网格可勾选、操作栏可用）
}

// MergeSettings 合并相关设置
type MergeSettings struct {
	Strategy string `json:"strategy" koanf:"strategy"` // 合并产物挂载策略：keep=新建 merged 保留原轨道 / overwrite=新建 merged 删原轨道
}

// FsmonitorSettings 工作目录监控设置
type FsmonitorSettings struct {
	UsnEnabled         bool              `json:"usnEnabled" koanf:"usnEnabled"`                 // USN 离线精确追溯开关（仅 Windows，需管理员运行）；默认关，离线走全量对账。详见 ../library-squirrel-docs/plan/USN离线追溯方案.md D8
	SuppressEnabled    bool              `json:"suppressEnabled" koanf:"suppressEnabled"`       // 操作抑制开关（D7）：默认开，关闭则 fsmonitor 不抑制内部写入（退回误报原状态，对账兜底）。详见 ../library-squirrel-docs/plan/store操作抑制suppression方案.md
	AutoRepairEnabled  bool              `json:"autoRepairEnabled" koanf:"autoRepairEnabled"`   // 自动修复模式开关（决策1）：默认关，用户显式开启。开启后 live 路径变更按策略自动处理，offline 一律人工确认。详见 ../library-squirrel-docs/plan/工作目录外部操作防护方案.md
	AutoRepairPolicies map[string]string `json:"autoRepairPolicies" koanf:"autoRepairPolicies"` // 自动修复策略覆盖表（key="<domain>:<kind>"，value=动作；未覆盖的组合回落内置默认，可选项由 fsmonitor 策略表约束）
}

// BackupGovernanceSettings 备份治理设置（治理常开无开关，仅保留期可配）
type BackupGovernanceSettings struct {
	RetentionDays int `json:"retentionDays" koanf:"retentionDays"` // 无主备份保留天数：清单行不被任何业务列引用且超期即清理
}

// ExportSettings 导出相关设置
type ExportSettings struct {
	// OutputDir 设置页显式配置的导出默认输出目录（空=沿用工作目录作为导出落盘根）；
	// 导出弹窗内的临时改选仅本次生效，不写回本字段
	OutputDir string `json:"outputDir" koanf:"outputDir"`
}

// ShareSettings 分享相关设置
type ShareSettings struct {
	// RelayAddress 分享中继地址（host 或 host:port，可带 https:// 前缀；无端口默认 9527）。
	// 官方中继占位默认值，可改为社区自建中继
	RelayAddress string `json:"relayAddress" koanf:"relayAddress"`
}

// DefaultShareRelayAddress 官方中继占位地址（正式部署前为占位值，用户可在设置页改指社区自建中继）
const DefaultShareRelayAddress = "relay.library-squirrel.cn"

// DefaultBackupGovernanceRetentionDays 无主备份保留天数默认值（7 天大于替换任务合理在途时长，
// 覆盖任意链崩溃/中断窗口；小于 1 的取值视为未配置/误配，回退此值——0 作为"立即清空"不被接受）
const DefaultBackupGovernanceRetentionDays = 7

// 合并策略取值
const (
	MergeStrategyKeep      = "keep"      // 新建 merged store，保留原 videoTrack/audioTrack
	MergeStrategyOverwrite = "overwrite" // 新建 merged store，原轨道 store 及文件转入回收站（可复原，到期自动清理）
)

// NewSettings 创建默认设置
func NewSettings() *Settings {
	return &Settings{
		WorkDir: "",
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
			Theme:              "default-light",
			MultiSelectEnabled: false,
		},
		MergeSettings: MergeSettings{
			Strategy: MergeStrategyKeep,
		},
		FsmonitorSettings: FsmonitorSettings{
			UsnEnabled:         false,
			SuppressEnabled:    true,
			AutoRepairEnabled:  false,
			AutoRepairPolicies: map[string]string{},
		},
		BackupGovernance: BackupGovernanceSettings{
			RetentionDays: DefaultBackupGovernanceRetentionDays,
		},
		ExportSettings: ExportSettings{},
		ShareSettings: ShareSettings{
			RelayAddress: DefaultShareRelayAddress,
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
