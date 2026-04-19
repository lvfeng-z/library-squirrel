package query

// QueryAttribute 查询属性：值 + 运算符 + 排序
// 用于各模块 QueryDTO 的字段定义
type QueryAttribute struct {
	Value    interface{} `json:"value"`              // 查询值
	Operator Operator    `json:"operator,omitempty"` // 运算符（可省略，默认 eq）
	Order    SortOrder   `json:"order,omitempty"`    // 排序（可省略）
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
