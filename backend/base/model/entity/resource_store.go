package entity

import (
	"github.com/library-squirrel/backend/base/model"
	"github.com/lvfeng-z/library-squirrel-sdk/contract"
)

// StoreType/Generation 常量别名 SDK contract 包（单一真相源，见 contract 包文档）。
const (
	StoreTypeImage      = contract.StoreTypeImage
	StoreTypeDocument   = contract.StoreTypeDocument
	StoreTypeThumbnail  = contract.StoreTypeThumbnail
	StoreTypeVideoTrack = contract.StoreTypeVideoTrack
	StoreTypeAudioTrack = contract.StoreTypeAudioTrack
	StoreTypeVideoMain  = contract.StoreTypeVideoMain

	GenerationDownloaded = contract.GenerationDownloaded
	GenerationDerived    = contract.GenerationDerived
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
