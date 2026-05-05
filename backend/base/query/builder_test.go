package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestQueryBuilder_BuildWhere_Nil(t *testing.T) {
	builder := &QueryBuilder{}
	result, err := builder.BuildWhere(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestQueryBuilder_BuildWhere_SimpleCondition(t *testing.T) {
	builder := &QueryBuilder{}

	dto := &WhereDTO{
		Type: "and",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "status", Operator: OpEq, Value: 1}},
		},
	}

	result, err := builder.BuildWhere(dto)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// 验证结果是 clause.Eq
	expr := result[0]
	eq, ok := expr.(clause.Eq)
	require.True(t, ok, "expected clause.Eq")
	assert.Equal(t, "status", string(eq.Column.(clause.Column).Name))
	assert.Equal(t, 1, eq.Value)
}

func TestQueryBuilder_BuildWhere_MultipleConditions(t *testing.T) {
	builder := &QueryBuilder{}

	dto := &WhereDTO{
		Type: "and",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "status", Operator: OpEq, Value: 1}},
			{Condition: &Condition{Field: "name", Operator: OpLike, Value: "test"}},
		},
	}

	result, err := builder.BuildWhere(dto)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// 验证结果是 AND 组合
	expr := result[0]
	and, ok := expr.(clause.AndConditions)
	require.True(t, ok, "expected clause.AndConditions")
	assert.Len(t, and.Exprs, 2)
}

func TestQueryBuilder_BuildWhere_OrCondition(t *testing.T) {
	builder := &QueryBuilder{}

	dto := &WhereDTO{
		Type: "or",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "name", Operator: OpEq, Value: "Alice"}},
			{Condition: &Condition{Field: "name", Operator: OpEq, Value: "Bob"}},
		},
	}

	result, err := builder.BuildWhere(dto)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// 验证结果是 OR 组合
	expr := result[0]
	or, ok := expr.(clause.OrConditions)
	require.True(t, ok, "expected clause.OrConditions")
	assert.Len(t, or.Exprs, 2)
}

func TestQueryBuilder_BuildWhere_NestedGroup(t *testing.T) {
	builder := &QueryBuilder{}

	// WHERE status = 1 AND (name = 'Alice' OR email LIKE '%@example.com%')
	dto := &WhereDTO{
		Type: "and",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "status", Operator: OpEq, Value: 1}},
			{
				Group: &WhereDTO{
					Type: "or",
					Conditions: []ConditionOrGroup{
						{Condition: &Condition{Field: "name", Operator: OpEq, Value: "Alice"}},
						{Condition: &Condition{Field: "email", Operator: OpLike, Value: "@example.com"}},
					},
				},
			},
		},
	}

	result, err := builder.BuildWhere(dto)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// 验证结果是 AND 组合
	expr := result[0]
	and, ok := expr.(clause.AndConditions)
	require.True(t, ok, "expected clause.AndConditions")
	assert.Len(t, and.Exprs, 2)

	// 第二个表达式应该是 OR
	or, ok := and.Exprs[1].(clause.OrConditions)
	require.True(t, ok, "expected nested clause.OrConditions")
	assert.Len(t, or.Exprs, 2)
}

func TestQueryBuilder_BuildWhere_AllOperators(t *testing.T) {
	builder := &QueryBuilder{}

	testCases := []struct {
		name     string
		operator Operator
		value    interface{}
	}{
		{"eq", OpEq, 1},
		{"ne", OpNe, 1},
		{"gt", OpGt, 10},
		{"gte", OpGte, 10},
		{"lt", OpLt, 10},
		{"lte", OpLte, 10},
		{"like", OpLike, "test"},
		{"in", OpIn, []interface{}{1, 2, 3}},
		{"is_null", OpIsNull, nil},
		{"is_not_null", OpIsNotNull, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dto := &WhereDTO{
				Type: "and",
				Conditions: []ConditionOrGroup{
					{Condition: &Condition{Field: "field", Operator: tc.operator, Value: tc.value}},
				},
			}

			result, err := builder.BuildWhere(dto)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.NotNil(t, result[0])
		})
	}
}

func TestQueryBuilder_BuildWhere_UnsupportedOperator(t *testing.T) {
	builder := &QueryBuilder{}

	dto := &WhereDTO{
		Type: "and",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "field", Operator: Operator("unsupported"), Value: 1}},
		},
	}

	result, err := builder.BuildWhere(dto)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported operator")
}

func TestQueryBuilder_BuildOrder_Nil(t *testing.T) {
	builder := &QueryBuilder{}
	result := builder.BuildOrder(nil)
	assert.Nil(t, result)
}

func TestQueryBuilder_BuildOrder_EmptySlice(t *testing.T) {
	builder := &QueryBuilder{}
	result := builder.BuildOrder([]OrderDTO{})
	assert.Nil(t, result)
}

func TestQueryBuilder_BuildOrder_SingleColumn(t *testing.T) {
	builder := &QueryBuilder{}

	dtos := []OrderDTO{
		{Field: "create_time", Order: OrderDesc},
	}

	result := builder.BuildOrder(dtos)
	require.NotNil(t, result)

	orderBy, ok := result.(clause.OrderBy)
	require.True(t, ok)
	require.Len(t, orderBy.Columns, 1)
	assert.Equal(t, "create_time", orderBy.Columns[0].Column.Name)
	assert.True(t, orderBy.Columns[0].Desc)
}

func TestQueryBuilder_BuildOrder_MultipleColumns(t *testing.T) {
	builder := &QueryBuilder{}

	dtos := []OrderDTO{
		{Field: "create_time", Order: OrderDesc},
		{Field: "update_time", Order: OrderAsc},
	}

	result := builder.BuildOrder(dtos)
	require.NotNil(t, result)

	orderBy, ok := result.(clause.OrderBy)
	require.True(t, ok)
	require.Len(t, orderBy.Columns, 2)

	// 第一个排序
	assert.Equal(t, "create_time", orderBy.Columns[0].Column.Name)
	assert.True(t, orderBy.Columns[0].Desc)

	// 第二个排序
	assert.Equal(t, "update_time", orderBy.Columns[1].Column.Name)
	assert.False(t, orderBy.Columns[1].Desc)
}

func TestQueryBuilder_BuildOrder_CaseInsensitive(t *testing.T) {
	builder := &QueryBuilder{}

	testCases := []struct {
		name  string
		order SortOrder
		desc  bool
	}{
		{"asc lowercase", OrderAsc, false},
		{"desc lowercase", OrderDesc, true},
		{"ASC uppercase", SortOrder("ASC"), false},
		{"DESC uppercase", SortOrder("DESC"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dtos := []OrderDTO{{Field: "field", Order: tc.order}}
			result := builder.BuildOrder(dtos)
			require.NotNil(t, result)
			orderBy := result.(clause.OrderBy)
			assert.Equal(t, tc.desc, orderBy.Columns[0].Desc)
		})
	}
}

func TestQueryBuilder_Integration(t *testing.T) {
	builder := &QueryBuilder{}

	// 完整查询 DTO
	dto := &WhereDTO{
		Type: "and",
		Conditions: []ConditionOrGroup{
			{Condition: &Condition{Field: "status", Operator: OpEq, Value: 1}},
			{
				Group: &WhereDTO{
					Type: "or",
					Conditions: []ConditionOrGroup{
						{Condition: &Condition{Field: "name", Operator: OpLike, Value: "Alice"}},
						{Condition: &Condition{Field: "email", Operator: OpLike, Value: "@example.com"}},
					},
				},
			},
		},
	}

	// 构建 WHERE
	whereExprs, err := builder.BuildWhere(dto)
	require.NoError(t, err)
	require.Len(t, whereExprs, 1)

	// 验证 AND 组合
	and := whereExprs[0].(clause.AndConditions)
	require.Len(t, and.Exprs, 2)

	// 第一个条件
	eq := and.Exprs[0].(clause.Eq)
	assert.Equal(t, "status", string(eq.Column.(clause.Column).Name))
	assert.Equal(t, 1, eq.Value)

	// 第二个条件（嵌套 OR）
	or := and.Exprs[1].(clause.OrConditions)
	require.Len(t, or.Exprs, 2)

	// 验证嵌套 LIKE
	like0 := or.Exprs[0].(clause.Like)
	assert.Equal(t, "name", string(like0.Column.(clause.Column).Name))
	assert.Equal(t, "%Alice%", like0.Value)

	like1 := or.Exprs[1].(clause.Like)
	assert.Equal(t, "email", string(like1.Column.(clause.Column).Name))
	assert.Equal(t, "%@example.com%", like1.Value)
}
