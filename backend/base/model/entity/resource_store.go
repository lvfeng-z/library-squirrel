package entity

import (
	"github.com/library-squirrel/backend/base/model"
)

// ==== StoreType 常量(封闭枚举)====
// 标识 resource_store 的业务角色;资源类型规约体系(ResourceTypeSpec)据此组合表达结构。
const (
	StoreTypeImage      = "image"      // 图片(image 资源主体;article 内嵌图多例)
	StoreTypeDocument   = "document"   // 文档文件(article 正文 .md;document 原文件 .pdf/.docx)
	StoreTypeThumbnail  = "thumbnail"  // 缩略图/封面
	StoreTypeVideoTrack = "videoTrack" // 视频轨(分离流视频原料)
	StoreTypeAudioTrack = "audioTrack" // 音频轨(分离流音频原料)
	StoreTypeVideoMain  = "videoMain"  // 视频可播放主体(封装原文件 downloaded 或合并产物 derived)
)

// ==== Generation 常量 ====
// 标识 store 实例的生成方式(数据获取时序),决定执行与续传语义。
// 这是 store 实例属性(每行 resource_store 一个值),由产出方(StoreSpec/MergeService/还原)决定,
// 不从 store_type 推断——同一 store_type 可跨 generation(如 videoMain:封装原文件 downloaded、合并产物 derived)。
const (
	GenerationDownloaded = "downloaded" // 流式下载获得,支持断点续传
	GenerationDerived    = "derived"    // 一次性派生产物,不可续传
)

// ResourceStore Resource 关联的 typed store:一个 Resource 挂 N 个 store,
// 每个 store 带 store_type(业务角色)与 generation(生成方式)。
type ResourceStore struct {
	*model.BaseEntity
	ResourceID int64  `gorm:"column:resource_id;index:idx_resource_store_resource_id" json:"resourceId"`
	StoreType  string `gorm:"column:store_type" json:"storeType"`
	Generation string `gorm:"column:generation" json:"generation"`
	StoreID    int64  `gorm:"column:store_id;index:idx_resource_store_store_id" json:"storeId"`
	StoreSeq   int    `gorm:"column:store_seq" json:"storeSeq"`
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
