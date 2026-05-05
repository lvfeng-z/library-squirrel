package query

import (
	"fmt"
	"strings"

	"gorm.io/gorm/clause"
)

// QueryBuilder 负责将 DTO 转换为 GORM Clauses
type QueryBuilder struct{}

// BuildWhere 将 WhereDTO 转换为 []clause.Expression
// 如果 WhereDTO 为 nil，返回空列表
func (b *QueryBuilder) BuildWhere(dto *WhereDTO) ([]clause.Expression, error) {
	if dto == nil {
		return nil, nil
	}

	expr, err := b.convertGroup(dto)
	if err != nil {
		return nil, err
	}

	if expr == nil {
		return nil, nil
	}

	return []clause.Expression{expr}, nil
}

// BuildOrder 将 OrderDTO 切片转换为 clause.OrderBy
// 返回 clause.OrderBy 表达式
func (b *QueryBuilder) BuildOrder(dtos []OrderDTO) clause.Expression {
	if len(dtos) == 0 {
		return nil
	}

	var columns []clause.OrderByColumn
	for _, dto := range dtos {
		desc := strings.ToLower(string(dto.Order)) == string(OrderDesc)
		columns = append(columns, clause.OrderByColumn{
			Column: clause.Column{Name: dto.Field},
			Desc:   desc,
		})
	}
	return clause.OrderBy{Columns: columns}
}

// convertGroup 递归处理 ConditionGroup
func (b *QueryBuilder) convertGroup(group *WhereDTO) (clause.Expression, error) {
	if group == nil {
		return nil, nil
	}

	var subExprs []clause.Expression

	for _, item := range group.Conditions {
		var expr clause.Expression
		var err error

		// 检查是否为嵌套组
		if item.Group != nil {
			expr, err = b.convertGroup(item.Group)
		} else if item.Condition != nil {
			expr, err = b.convertCondition(*item.Condition)
		} else {
			continue
		}

		if err != nil {
			return nil, err
		}

		if expr != nil {
			subExprs = append(subExprs, expr)
		}
	}

	if len(subExprs) == 0 {
		return nil, nil
	}

	switch strings.ToLower(group.Type) {
	case "or":
		return clause.Or(subExprs...), nil
	default: // "and" 或默认
		return clause.And(subExprs...), nil
	}
}

// convertCondition 将 Condition 转换为 clause.Expression
func (b *QueryBuilder) convertCondition(c Condition) (clause.Expression, error) {
	col := clause.Column{Name: c.Field}

	switch c.Operator {
	case OpEq:
		return clause.Eq{Column: col, Value: c.Value}, nil
	case OpNe:
		return clause.Neq{Column: col, Value: c.Value}, nil
	case OpGt:
		return clause.Gt{Column: col, Value: c.Value}, nil
	case OpGte:
		return clause.Gte{Column: col, Value: c.Value}, nil
	case OpLt:
		return clause.Lt{Column: col, Value: c.Value}, nil
	case OpLte:
		return clause.Lte{Column: col, Value: c.Value}, nil
	case OpLike:
		return b.buildLike(c, col)
	case OpIn:
		return clause.IN{Column: col, Values: toValues(c.Value)}, nil
	case OpIsNull:
		return clause.Expr{SQL: "? IS NULL", Vars: []interface{}{col}}, nil
	case OpIsNotNull:
		return clause.Expr{SQL: "? IS NOT NULL", Vars: []interface{}{col}}, nil
	default:
		return nil, fmt.Errorf("unsupported operator: %s", c.Operator)
	}
}

// buildLike 构建 LIKE 表达式，自动处理通配符
func (b *QueryBuilder) buildLike(c Condition, col clause.Column) (clause.Expression, error) {
	valStr := fmt.Sprintf("%v", c.Value)
	if !strings.Contains(valStr, "%") {
		valStr = "%" + valStr + "%"
	}
	return clause.Like{Column: col, Value: valStr}, nil
}

// toValues 将 interface{} 转换为 []interface{}
// 用于处理 IN 操作符的值
func toValues(v interface{}) []interface{} {
	if vals, ok := v.([]interface{}); ok {
		return vals
	}
	// 简单处理：如果不是切片，转为单元素切片
	return []interface{}{v}
}
