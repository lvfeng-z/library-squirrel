package entity

import (
	"errors"
)

// ResourceType* 预定义资源类型常量(封闭枚举,主程序中央注册)。
// 插件创建资源时必须声明其一;主程序不推断、不兜底,空/未知值在写入路径抛错。
const (
	ResourceTypeImage    = "image"    // 图片资源(单图/多图每子资源)
	ResourceTypeVideo    = "video"    // 视频资源(视频轨+音频轨,可合并)
	ResourceTypeArticle  = "article"  // 图文紧密结合文档(正文 markdown + 内嵌图)
	ResourceTypeDocument = "document" // 现成文档原文件(pdf/docx/...)
	ResourceTypeUnknown  = "unknown"  // 合法显式值:插件确实无法分类时声明
)

// 写入路径严格识别错误
var (
	ErrResourceTypeEmpty   = errors.New("资源类型未声明")
	ErrResourceTypeInvalid = errors.New("资源类型非预定义值")
	ErrStoreTypeInvalid    = errors.New("store_type 非预定义值")
)

// StoreRoleSpec 结构角色基数(完整性校验用)。
// Min=最少数量(0=可选,1=必含);Max=最多数量(0=不限,1=单例)。
type StoreRoleSpec struct {
	StoreType string
	Min       int
	Max       int
}

// StoreStandard 角色的文件标准(描述性,不做内容校验)。
type StoreStandard struct {
	Description string   // 角色用途说明
	Formats     []string // 期望文件扩展名(描述性,非强制)
	Generation  string   // downloaded(下载) | derived(派生)
}

// ResourceTypeSpec 资源类型规约:该类型资源的结构角色组合(含基数)、
// 展示主体优先级、各角色文件标准。
type ResourceTypeSpec struct {
	ResourceType   string                  // 资源类型值(对应 ResourceType* 常量)
	Roles          []StoreRoleSpec         // 结构角色 + 基数(完整性校验)
	PrimaryRoles   []string                // 展示主体优先级链(高→低,ResolvePrimaryStore 取第一个实际存在)
	StoreStandards map[string]StoreStandard // key=StoreType
}

// ResourceTypeRegistry 资源类型中央注册表(封闭枚举)。
// 主程序预定义 5 种资源类型;插件暂不可自定义。
var ResourceTypeRegistry = map[string]ResourceTypeSpec{
	ResourceTypeImage:    imageResourceTypeSpec,
	ResourceTypeVideo:    videoResourceTypeSpec,
	ResourceTypeArticle:  articleResourceTypeSpec,
	ResourceTypeDocument: documentResourceTypeSpec,
	ResourceTypeUnknown:  unknownResourceTypeSpec,
}

// validStoreTypes 合法的 store_type 集合(写入路径严格识别用)
var validStoreTypes = map[string]struct{}{
	StoreTypeImage:      {},
	StoreTypeDocument:   {},
	StoreTypeThumbnail:  {},
	StoreTypeVideoTrack: {},
	StoreTypeAudioTrack: {},
	StoreTypeMerged:     {},
}

var imageResourceTypeSpec = ResourceTypeSpec{
	ResourceType: ResourceTypeImage,
	Roles: []StoreRoleSpec{
		{StoreType: StoreTypeImage, Min: 1, Max: 1},
		{StoreType: StoreTypeThumbnail, Min: 0, Max: 1},
	},
	PrimaryRoles: []string{StoreTypeImage},
	StoreStandards: map[string]StoreStandard{
		StoreTypeImage:     {Description: "图片主体", Formats: []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}, Generation: GenerationDownloaded},
		StoreTypeThumbnail: {Description: "封面/缩略图", Formats: []string{".jpg", ".png", ".webp"}, Generation: GenerationDerived},
	},
}

var videoResourceTypeSpec = ResourceTypeSpec{
	ResourceType: ResourceTypeVideo,
	Roles: []StoreRoleSpec{
		{StoreType: StoreTypeVideoTrack, Min: 1, Max: 1},
		{StoreType: StoreTypeAudioTrack, Min: 1, Max: 1},
		{StoreType: StoreTypeThumbnail, Min: 0, Max: 1},
		{StoreType: StoreTypeMerged, Min: 0, Max: 1},
	},
	PrimaryRoles: []string{StoreTypeMerged, StoreTypeVideoTrack},
	StoreStandards: map[string]StoreStandard{
		StoreTypeVideoTrack: {Description: "视频流", Formats: []string{".mp4"}, Generation: GenerationDownloaded},
		StoreTypeAudioTrack: {Description: "音频流", Formats: []string{".m4a", ".mp3", ".aac"}, Generation: GenerationDownloaded},
		StoreTypeThumbnail:  {Description: "封面/缩略图", Formats: []string{".jpg", ".png", ".webp"}, Generation: GenerationDerived},
		StoreTypeMerged:     {Description: "视频+音频合并产物", Formats: []string{".mp4"}, Generation: GenerationDerived},
	},
}

var articleResourceTypeSpec = ResourceTypeSpec{
	ResourceType: ResourceTypeArticle,
	Roles: []StoreRoleSpec{
		{StoreType: StoreTypeDocument, Min: 1, Max: 1},
		{StoreType: StoreTypeImage, Min: 0, Max: 0}, // 内嵌图 0~N(Max=0 表示不限)
		{StoreType: StoreTypeThumbnail, Min: 0, Max: 1},
	},
	PrimaryRoles: []string{StoreTypeDocument},
	StoreStandards: map[string]StoreStandard{
		StoreTypeDocument:  {Description: "专栏正文(markdown)", Formats: []string{".md"}, Generation: GenerationDerived},
		StoreTypeImage:     {Description: "内嵌图", Formats: []string{".jpg", ".png", ".webp"}, Generation: GenerationDownloaded},
		StoreTypeThumbnail: {Description: "封面/缩略图", Formats: []string{".jpg", ".png", ".webp"}, Generation: GenerationDerived},
	},
}

var documentResourceTypeSpec = ResourceTypeSpec{
	ResourceType: ResourceTypeDocument,
	Roles: []StoreRoleSpec{
		{StoreType: StoreTypeDocument, Min: 1, Max: 1},
		{StoreType: StoreTypeThumbnail, Min: 0, Max: 1},
	},
	PrimaryRoles: []string{StoreTypeDocument},
	StoreStandards: map[string]StoreStandard{
		StoreTypeDocument:  {Description: "现成文档原文件", Formats: []string{".pdf", ".docx", ".doc", ".txt", ".rtf"}, Generation: GenerationDownloaded},
		StoreTypeThumbnail: {Description: "封面/缩略图", Formats: []string{".jpg", ".png", ".webp"}, Generation: GenerationDerived},
	},
}

// unknownResourceTypeSpec 合法显式值:插件确实无法分类时声明。
// 无结构约束、无展示主体,前端走扩展名嗅探兜底。
var unknownResourceTypeSpec = ResourceTypeSpec{
	ResourceType:   ResourceTypeUnknown,
	Roles:          nil,
	PrimaryRoles:   nil,
	StoreStandards: nil,
}

// LookupResourceTypeSpec 查询资源类型规约;未注册(含空字符串)返回 nil。
func LookupResourceTypeSpec(resourceType string) *ResourceTypeSpec {
	spec, ok := ResourceTypeRegistry[resourceType]
	if !ok {
		return nil
	}
	return &spec
}

// ValidateResourceStructure 校验资源结构完整性:每角色实际数量 ∈ [Min,Max]。
// 纯集合运算,无 IO。resourceType 未注册(含空/unknown)→ 不校验,返回空。
// 返回 missing(必含角色缺失或不足)、excess(角色超量)。
func ValidateResourceStructure(resourceType string, storeTypeCounts map[string]int) (missing, excess []string) {
	spec, ok := ResourceTypeRegistry[resourceType]
	if !ok {
		return nil, nil
	}
	for _, role := range spec.Roles {
		count := storeTypeCounts[role.StoreType]
		if count < role.Min {
			missing = append(missing, role.StoreType)
		}
		if role.Max > 0 && count > role.Max {
			excess = append(excess, role.StoreType)
		}
	}
	return missing, excess
}

// ResolvePrimaryStore 按 spec.PrimaryRoles 优先级链取第一个实际存在的 store 作为展示主体。
// spec 为 nil 或 PrimaryRoles 为空(unknown/未注册 resource_type)时安全降级:
// merged(完整可播放/查看)→ image(图片主体)→ 首个非 thumbnail,保证历史/未知资源展示不回归。
// 读路径专用,不抛错。
func ResolvePrimaryStore(stores []*ResourceStore, spec *ResourceTypeSpec) *ResourceStore {
	if spec != nil {
		for _, primary := range spec.PrimaryRoles {
			for _, s := range stores {
				if s != nil && s.StoreType == primary {
					return s
				}
			}
		}
	}
	for _, s := range stores {
		if s != nil && s.StoreType == StoreTypeMerged {
			return s
		}
	}
	for _, s := range stores {
		if s != nil && s.StoreType == StoreTypeImage {
			return s
		}
	}
	for _, s := range stores {
		if s != nil && s.StoreType != StoreTypeThumbnail {
			return s
		}
	}
	return nil
}

// ValidateResourceType 严格识别资源类型(写入路径用)。
// 空值→ErrResourceTypeEmpty;非预定义值(不在 Registry)→ErrResourceTypeInvalid。
// unknown 是合法显式值(在 Registry 中),不报错。
func ValidateResourceType(resourceType string) error {
	if resourceType == "" {
		return ErrResourceTypeEmpty
	}
	if _, ok := ResourceTypeRegistry[resourceType]; !ok {
		return ErrResourceTypeInvalid
	}
	return nil
}

// ValidateStoreType 严格识别 store_type(写入路径用)。非 6 预定义角色→ErrStoreTypeInvalid。
func ValidateStoreType(storeType string) error {
	if _, ok := validStoreTypes[storeType]; !ok {
		return ErrStoreTypeInvalid
	}
	return nil
}
