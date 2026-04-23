package model

// Entity 实体接口，所有领域实体必须实现此接口
type Entity interface {
	GetID() int64
	SetID(id int64)
	GetCreateTime() int64
	SetCreateTime(time int64)
	GetUpdateTime() int64
	SetUpdateTime(time int64)
}

// BaseEntity 基础实体结构体，所有领域实体通过嵌入此结构体获得公共字段和方法
type BaseEntity struct {
	ID         int64 `gorm:"primaryKey;column:id" json:"id"`
	CreateTime int64 `gorm:"column:create_time" json:"createTime"`
	UpdateTime int64 `gorm:"column:update_time" json:"updateTime"`
}

// GetID 实现 Entity 接口
func (b *BaseEntity) GetID() int64 {
	return b.ID
}

// SetID 实现 Entity 接口
func (b *BaseEntity) SetID(id int64) {
	b.ID = id
}

// GetCreateTime 实现 Entity 接口
func (b *BaseEntity) GetCreateTime() int64 {
	return b.CreateTime
}

// SetCreateTime 实现 Entity 接口
func (b *BaseEntity) SetCreateTime(time int64) {
	b.CreateTime = time
}

// GetUpdateTime 实现 Entity 接口
func (b *BaseEntity) GetUpdateTime() int64 {
	return b.UpdateTime
}

// SetUpdateTime 实现 Entity 接口
func (b *BaseEntity) SetUpdateTime(time int64) {
	b.UpdateTime = time
}

// Page 分页响应（与渲染进程 IPage 保持一致）
type Page[D any, Q any] struct {
	// 当前页码
	PageNumber int `json:"pageNumber"`
	// 分页大小
	PageSize int `json:"pageSize"`
	// 总页数
	PageCount int `json:"pageCount"`
	// 数据总量
	DataCount int64 `json:"dataCount"`
	// 本页数据量
	CurrentCount int `json:"currentCount"`
	// 查询条件
	Query Q `json:"query,omitempty"`
	// 数据列表
	Data []*D `json:"data"`
}

// NewPage 创建分页响应
func NewPage[D any, Q any](data []*D, total int64, pageNumber, pageSize int) *Page[D, Q] {
	pageCount := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pageCount++
	}
	if pageCount < 1 {
		pageCount = 1
	}
	return &Page[D, Q]{
		PageNumber:   pageNumber,
		PageSize:     pageSize,
		PageCount:    pageCount,
		DataCount:    total,
		CurrentCount: len(data),
		Data:         data,
	}
}
