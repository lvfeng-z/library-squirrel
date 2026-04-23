package query

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm/clause"

	"github.com/library-squirrel/wails/internal/database"
)

// Converter 结构体 → PageOption/QueryOption 转换器
type Converter struct {
	mapper *FieldMapper
}

// NewConverter 创建转换器
func NewConverter(model interface{}) *Converter {
	return &Converter{
		mapper: NewFieldMapper(model),
	}
}

// ToQueryOption 将 DTO 转换为 QueryOption
// 只处理数据库字段，非数据库字段会被跳过
func (c *Converter) ToQueryOption(dto interface{}) (*database.QueryOption, error) {
	fields, err := c.mapper.CollectDbFields(dto)
	if err != nil {
		return nil, err
	}

	var conditions []clause.Expression
	var orderColumns []clause.OrderByColumn

	// 先收集所有有排序的字段
	type orderField struct {
		columnName string
		desc       bool
		priority   int
	}
	var orders []orderField

	for _, field := range fields {
		// 跳过非数据库字段
		if !field.IsDbField || field.QueryAttr == nil {
			continue
		}

		attr := field.QueryAttr

		// 处理排序（即使没有查询值，只要指定了排序就加入）
		if attr.HasOrder() {
			orders = append(orders, orderField{
				columnName: field.ColumnName,
				desc:       attr.Order == OrderDesc,
				priority:   attr.Priority,
			})
		}

		// 处理查询条件（需要有效值）
		if attr.Value == nil {
			continue
		}

		// 构建条件
		cond := Condition{
			Field:    field.ColumnName,
			Operator: attr.GetOperator(),
			Value:    attr.Value,
		}
		expr, err := c.buildCondition(cond)
		if err != nil {
			return nil, err
		}
		if expr != nil {
			conditions = append(conditions, expr)
		}
	}

	// 按 Priority 排序并构建 orderColumns
	if len(orders) > 0 {
		// 按 priority 从小到大排序（priority 越小优先级越高）
		sort.Slice(orders, func(i, j int) bool {
			return orders[i].priority < orders[j].priority
		})
		for _, o := range orders {
			orderColumns = append(orderColumns, clause.OrderByColumn{
				Column: clause.Column{Name: o.columnName},
				Desc:   o.desc,
			})
		}
	}

	// 构建 WHERE 和 ORDER BY
	var where clause.Expression
	if len(conditions) > 0 {
		andCond := clause.AndConditions{}
		for _, cond := range conditions {
			andCond.Exprs = append(andCond.Exprs, cond)
		}
		where = andCond
	}

	var order clause.Expression
	if len(orderColumns) > 0 {
		order = clause.OrderBy{Columns: orderColumns}
	}

	var conditionList []clause.Expression
	if where != nil {
		conditionList = []clause.Expression{where}
	}

	var orderList []clause.Expression
	if order != nil {
		orderList = []clause.Expression{order}
	}

	return &database.QueryOption{
		Conditions: conditionList,
		OrderBy:    orderList,
	}, nil
}

// ToPageOption 将 DTO 转换为 PageOption
func (c *Converter) ToPageOption(dto interface{}, page, pageSize int) (*database.PageOption, error) {
	queryOpt, err := c.ToQueryOption(dto)
	if err != nil {
		return nil, err
	}

	return &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: queryOpt.Conditions,
			OrderBy:    queryOpt.OrderBy,
		},
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// buildCondition 构建单个查询条件
func (c *Converter) buildCondition(cond Condition) (clause.Expression, error) {
	col := clause.Column{Name: cond.Field}

	switch cond.Operator {
	case OpEq:
		return clause.Eq{Column: col, Value: cond.Value}, nil
	case OpNe:
		return clause.Neq{Column: col, Value: cond.Value}, nil
	case OpGt:
		return clause.Gt{Column: col, Value: cond.Value}, nil
	case OpGte:
		return clause.Gte{Column: col, Value: cond.Value}, nil
	case OpLt:
		return clause.Lt{Column: col, Value: cond.Value}, nil
	case OpLte:
		return clause.Lte{Column: col, Value: cond.Value}, nil
	case OpLike:
		return c.buildLike(cond, col)
	case OpIn:
		return clause.IN{Column: col, Values: toInterfaceSlice(cond.Value)}, nil
	case OpIsNull:
		return clause.Expr{SQL: "? IS NULL", Vars: []interface{}{col}}, nil
	case OpIsNotNull:
		return clause.Expr{SQL: "? IS NOT NULL", Vars: []interface{}{col}}, nil
	default:
		return nil, nil
	}
}

// buildLike 构建 LIKE 表达式，自动处理通配符
func (c *Converter) buildLike(cond Condition, col clause.Column) (clause.Expression, error) {
	valStr := fmt.Sprintf("%v", cond.Value)
	if !strings.Contains(valStr, "%") {
		valStr = "%" + valStr + "%"
	}
	return clause.Like{Column: col, Value: valStr}, nil
}

// toInterfaceSlice 将值转换为 []interface{}
func toInterfaceSlice(v interface{}) []interface{} {
	if vals, ok := v.([]interface{}); ok {
		return vals
	}
	return []interface{}{v}
}
