package query

import (
	"encoding/json"
)

// Operator 定义支持的查询操作符
type Operator string

const (
	OpEq        Operator = "eq"          // =
	OpNe        Operator = "ne"          // !=
	OpGt        Operator = "gt"          // >
	OpGte       Operator = "gte"         // >=
	OpLt        Operator = "lt"          // <
	OpLte       Operator = "lte"         // <=
	OpLike      Operator = "like"        // LIKE
	OpIn        Operator = "in"          // IN
	OpIsNull    Operator = "is_null"     // IS NULL
	OpIsNotNull Operator = "is_not_null" // IS NOT NULL
)

// SortOrder 定义排序顺序
type SortOrder string

const (
	OrderAsc  SortOrder = "asc"
	OrderDesc SortOrder = "desc"
)

// Condition 定义单个查询条件
type Condition struct {
	Field    string      `json:"field"`
	Operator Operator    `json:"operator"`
	Value    interface{} `json:"value"`
}

// WhereDTO 用于 WHERE 子句的 DTO，支持嵌套的 AND/OR 逻辑组合
type WhereDTO struct {
	Type       string             `json:"type"`       // "and" 或 "or"
	Conditions []ConditionOrGroup `json:"conditions"` // 条件或嵌套组
}

// ConditionOrGroup 条件或条件组的联合类型
// 在 JSON 反序列化时自动区分
type ConditionOrGroup struct {
	Condition *Condition `json:"-"`
	Group     *WhereDTO  `json:"-"`
}

// MarshalJSON 实现 ConditionOrGroup 的 JSON 序列化
func (c *ConditionOrGroup) MarshalJSON() ([]byte, error) {
	if c.Condition != nil {
		return json.Marshal(c.Condition)
	}
	if c.Group != nil {
		return json.Marshal(c.Group)
	}
	return []byte("null"), nil
}

// UnmarshalJSON 实现 ConditionOrGroup 的 JSON 反序列化
// 自动判断是 Condition 还是嵌套的 WhereDTO
func (c *ConditionOrGroup) UnmarshalJSON(data []byte) error {
	// 先尝试解析为 Condition
	var cond Condition
	if err := json.Unmarshal(data, &cond); err == nil {
		// 检查是否有 type 和 conditions 字段（表明是嵌套的 WhereDTO）
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err == nil {
			if _, hasType := m["type"]; hasType {
				if _, hasConditions := m["conditions"]; hasConditions {
					// 实际上是嵌套的 WhereDTO
					var group WhereDTO
					if err := json.Unmarshal(data, &group); err != nil {
						return err
					}
					c.Group = &group
					return nil
				}
			}
		}
		c.Condition = &cond
		return nil
	}

	// 尝试解析为 WhereDTO
	var group WhereDTO
	if err := json.Unmarshal(data, &group); err != nil {
		return err
	}
	c.Group = &group
	return nil
}

// OrderDTO 对应 ORDER BY 子句的 DTO
type OrderDTO struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}

// QueryDTO 完整查询 DTO（前端使用）
type QueryDTO struct {
	Where *WhereDTO  `json:"where,omitempty"`
	Order []OrderDTO `json:"order,omitempty"`
}
