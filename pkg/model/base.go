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

// ========== 分页模型 ==========

// PageRequest 分页请求
type PageRequest struct {
	// 当前页码
	PageNumber int `json:"pageNumber"`
	// 分页大小
	PageSize int `json:"pageSize"`
	// 查询条件
	Query interface{} `json:"query,omitempty"`
}

func (p *PageRequest) GetOffset() int {
	if p.PageNumber <= 0 {
		p.PageNumber = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 10
	}
	return (p.PageNumber - 1) * p.PageSize
}

func (p *PageRequest) GetLimit() int {
	if p.PageSize <= 0 {
		return 10
	}
	return p.PageSize
}

// ToExample 将 Query 转换为 Example
// Query 格式: map[string]interface{}{"field": value}
// 支持的操作符前缀: "", "!", ">", "<", ">=", "<=", "~" (LIKE)
// 示例: {"name": "test"} -> WHERE name = 'test'
//
//	{"name!": "test"} -> WHERE name != 'test'
//	{"name~": "%test%"} -> WHERE name LIKE '%test%'
func (p *PageRequest) ToExample() *Example {
	example := NewExample()

	if p.Query == nil {
		return example
	}

	queryMap, ok := p.Query.(map[string]interface{})
	if !ok {
		return example
	}

	// 特殊字段，不作为查询条件
	nonQueryFields := map[string]bool{
		"sort":     true,
		"page":     true,
		"pageSize": true,
	}

	for field, value := range queryMap {
		if nonQueryFields[field] {
			continue
		}

		// 解析操作符
		op := "="
		actualField := field

		if len(field) > 0 {
			switch field[0] {
			case '!':
				op = "!="
				actualField = field[1:]
			case '>':
				if len(field) > 1 && field[1] == '=' {
					op = ">="
					actualField = field[2:]
				} else {
					op = ">"
					actualField = field[1:]
				}
			case '<':
				if len(field) > 1 && field[1] == '=' {
					op = "<="
					actualField = field[2:]
				} else {
					op = "<"
					actualField = field[1:]
				}
			case '~':
				op = "LIKE"
				actualField = field[1:]
			}
		}

		// 跳过空值
		if value == nil {
			continue
		}

		example.WithCondition(actualField, op, value)
	}

	// 处理排序
	if sort, ok := queryMap["sort"]; ok {
		if sortList, ok := sort.([]interface{}); ok {
			for _, s := range sortList {
				if sortMap, ok := s.(map[string]interface{}); ok {
					key, _ := sortMap["key"].(string)
					asc, _ := sortMap["asc"].(bool)
					if key != "" {
						example.WithOrder(key, asc)
					}
				}
			}
		}
	}

	return example
}

// Page 分页响应（与渲染进程 IPage 保持一致）
type Page[T any] struct {
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
	Query interface{} `json:"query,omitempty"`
	// 数据列表
	Data []*T `json:"data"`
}

// NewPage 创建分页响应
func NewPage[T any](data []*T, total int64, pageNumber, pageSize int) *Page[T] {
	pageCount := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pageCount++
	}
	if pageCount < 1 {
		pageCount = 1
	}
	return &Page[T]{
		PageNumber:   pageNumber,
		PageSize:     pageSize,
		PageCount:    pageCount,
		DataCount:    total,
		CurrentCount: len(data),
		Data:         data,
	}
}
