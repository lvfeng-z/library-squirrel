package entity

import (
	"github.com/library-squirrel/backend/base/model"
)

// ==== StoreType 常量(开放枚举)====
// 标识 resource_store 的业务角色;新增资源类型时在此追加常量,不改表结构。
const (
	StoreTypeMain       = "main"       // 主资源
	StoreTypeThumbnail  = "thumbnail"  // 缩略图/封面
	StoreTypeVideoTrack = "videoTrack" // 视频轨
	StoreTypeAudioTrack = "audioTrack" // 音频轨
	StoreTypeMerged     = "merged"     // 合并产物
)

// ==== Generation 常量 ====
// 标识 store 的生成方式,决定执行与续传语义。
const (
	GenerationDownloaded = "downloaded" // 流式下载,支持断点续传
	GenerationDerived    = "derived"    // 一次性派生,不可续传
)

// ResourceStore Resource 关联的 typed store:一个 Resource 挂 N 个 store,
// 每个 store 带 store_type(业务角色)与 generation(生成方式)。
type ResourceStore struct {
	*model.BaseEntity
	ResourceID int64  `gorm:"column:resource_id;index:idx_resource_store_resource_id" json:"resourceId"`
	StoreType  string `gorm:"column:store_type" json:"storeType"`
	Generation string `gorm:"column:generation" json:"generation"`
	StoreID    int64  `gorm:"column:store_id;index:idx_resource_store_store_id" json:"storeId"`
	OrderIdx   int    `gorm:"column:order_idx" json:"orderIdx"`
}

// TableName 指定表名
func (ResourceStore) TableName() string {
	return "resource_store"
}

// NewResourceStore 创建 resource_store 记录
func NewResourceStore() *ResourceStore {
	return &ResourceStore{
		BaseEntity: &model.BaseEntity{},
	}
}
