package query

// QueryAttribute 查询属性：值 + 运算符 + 排序
// 用于各模块 QueryDTO 的字段定义
type QueryAttribute struct {
	Value    interface{} `json:"value"`              // 查询值
	Operator Operator    `json:"operator,omitempty"` // 运算符（可省略，默认 eq）
	Order    SortOrder   `json:"order,omitempty"`    // 排序方向（asc/desc）
	Priority int         `json:"priority,omitempty"` // 排序优先级，数字越小优先级越高
}

// IsEmpty 判断查询属性是否为空（无值且无排序）
func (a *QueryAttribute) IsEmpty() bool {
	return a.Value == nil && a.Order == ""
}

// GetOperator 获取运算符，默认返回 eq
func (a *QueryAttribute) GetOperator() Operator {
	if a.Operator == "" {
		return OpEq
	}
	return a.Operator
}

// HasOrder 判断是否需要排序
func (a *QueryAttribute) HasOrder() bool {
	return a.Order != ""
}

// HasPriority 判断是否有排序优先级（Priority >= 0 表示有效）
func (a *QueryAttribute) HasPriority() bool {
	return a.Priority >= 0
}
