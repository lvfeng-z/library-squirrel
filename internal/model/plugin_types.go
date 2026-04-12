package model

// InstallType 安装类型
type InstallType int

const (
	// InstallTypeManual 手动安装
	InstallTypeManual InstallType = 0
	// InstallTypeAuto 自动安装
	InstallTypeAuto InstallType = 1
)

// ActivationType 激活类型
type ActivationType int

const (
	// ActivationTypeManual 手动激活
	ActivationTypeManual ActivationType = 0
	// ActivationTypeStartup 启动时激活
	ActivationTypeStartup ActivationType = 1
)

// PluginActivation 插件激活配置
type PluginActivation struct {
	Type ActivationType `json:"type"`
}

// PluginContribute 插件贡献点
type PluginContribute struct {
	Type string `json:"type"` // taskHandler, slot, siteBrowser 等
	ID   string `json:"id"`
}

// PluginManifest 插件清单（从 plugin.json 解析）
type PluginManifest struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Author      string             `json:"author"`
	Description string             `json:"description,omitempty"`
	Contributes []PluginContribute `json:"contributes"`
	Activation  PluginActivation   `json:"activation"`
	EntryFile   string             `json:"entryFile"`
}

// PluginInstallDTO 插件安装数据传输对象
type PluginInstallDTO struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Author      string             `json:"author"`
	Description string             `json:"description,omitempty"`
	Contributes []PluginContribute `json:"contributes"`
	Activation  PluginActivation   `json:"activation"`
	EntryFile   string             `json:"entryFile"`
	PackagePath string             `json:"packagePath,omitempty"`
	PublicID    string             `json:"publicId,omitempty"`
}

// GetPublicID 获取插件公开ID（格式：作者/名称）
func (p *PluginManifest) GetPublicID() string {
	return p.Author + "/" + p.ID
}

// ToPluginInstallDTO 转换为安装DTO
func (p *PluginManifest) ToPluginInstallDTO(packagePath string) *PluginInstallDTO {
	return &PluginInstallDTO{
		ID:          p.ID,
		Name:        p.Name,
		Version:     p.Version,
		Author:      p.Author,
		Description: p.Description,
		Contributes: p.Contributes,
		Activation:  p.Activation,
		EntryFile:   p.EntryFile,
		PackagePath: packagePath,
		PublicID:    p.GetPublicID(),
	}
}

// NewPluginManifest 创建插件清单
func NewPluginManifest() *PluginManifest {
	return &PluginManifest{}
}
