package model

// Condition 查询条件
type Condition struct {
	Field string      `json:"field"` // 字段名
	Op    string      `json:"op"`    // 操作符: =, !=, >, <, >=, <=, LIKE, IN, IS_NULL, IS_NOT_NULL, BETWEEN
	Value interface{} `json:"value"` // 值
}

// OrderField 排序字段
type OrderField struct {
	Field string `json:"field"` // 字段名
	Asc   bool   `json:"asc"`   // 是否升序
}

// Example 查询条件构建器
type Example struct {
	Where   []Condition  `json:"where"`   // AND 条件
	Or      []Condition  `json:"or"`      // OR 条件
	OrderBy []OrderField `json:"orderBy"` // 排序
	Limit   int          `json:"limit"`   // 限制数量
	Offset  int          `json:"offset"`  // 偏移量
}

// NewExample 创建空Example
func NewExample() *Example {
	return &Example{}
}

// WithCondition 添加AND条件
func (e *Example) WithCondition(field, op string, value interface{}) *Example {
	e.Where = append(e.Where, Condition{Field: field, Op: op, Value: value})
	return e
}

// WithOrCondition 添加OR条件
func (e *Example) WithOrCondition(field, op string, value interface{}) *Example {
	e.Or = append(e.Or, Condition{Field: field, Op: op, Value: value})
	return e
}

// WithOrder 添加排序
func (e *Example) WithOrder(field string, asc bool) *Example {
	e.OrderBy = append(e.OrderBy, OrderField{Field: field, Asc: asc})
	return e
}

// WithLimit 添加限制
func (e *Example) WithLimit(limit int) *Example {
	e.Limit = limit
	return e
}

// WithOffset 添加偏移量
func (e *Example) WithOffset(offset int) *Example {
	e.Offset = offset
	return e
}

// Paginate 添加分页
func (e *Example) Paginate(page, pageSize int) *Example {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	e.Offset = (page - 1) * pageSize
	e.Limit = pageSize
	return e
}
