package query

type QueryAttributeInterface interface {
	// IsEmpty 判断查询属性是否为空（无值且无排序）
	IsEmpty() bool

	// GetValue 获取值
	GetValue() any

	// GetOperator 获取运算符，默认返回 eq
	GetOperator() Operator

	// GetOrder 获取排序
	GetOrder() SortOrder

	// GetPriority 获取排序优先级
	GetPriority() int

	// HasOrder 判断是否需要排序
	HasOrder() bool

	// HasPriority 判断是否有排序优先级
	HasPriority() bool
}

// QueryAttribute 查询属性：值 + 运算符 + 排序
// 用于各模块 QueryDTO 的字段定义
type QueryAttribute[T any] struct {
	Value    *T         `json:"value"`              // 查询值
	Operator *Operator  `json:"operator,omitempty"` // 运算符（可省略，默认 eq）
	Order    *SortOrder `json:"order,omitempty"`    // 排序方向（asc/desc）
	Priority *int       `json:"priority,omitempty"` // 排序优先级，数字越小优先级越高
}

// IsEmpty 判断查询属性是否为空（无值且无排序）
func (a QueryAttribute[T]) IsEmpty() bool {
	return a.Value == nil && a.Order == nil
}

// GetValue 获取值
func (a QueryAttribute[T]) GetValue() any {
	return a.Value
}

// GetOperator 获取运算符，默认返回 eq
func (a QueryAttribute[T]) GetOperator() Operator {
	if a.Operator == nil {
		return OpEq
	}
	return *a.Operator
}

// GetOrder 获取排序
func (a QueryAttribute[T]) GetOrder() SortOrder {
	return *a.Order
}

// GetPriority 获取排序优先级
func (a QueryAttribute[T]) GetPriority() int {
	return *a.Priority
}

// HasOrder 判断是否需要排序
func (a QueryAttribute[T]) HasOrder() bool {
	return a.Order != nil
}

// HasPriority 判断是否有排序优先级
func (a QueryAttribute[T]) HasPriority() bool {
	return a.Priority != nil
}
