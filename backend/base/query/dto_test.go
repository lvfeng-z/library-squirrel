package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCondition_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Condition
		wantErr  bool
	}{
		{
			name:  "simple eq condition",
			input: `{"field": "status", "operator": "eq", "value": 1}`,
			expected: Condition{
				Field:    "status",
				Operator: OpEq,
				Value:    float64(1),
			},
			wantErr: false,
		},
		{
			name:  "string value",
			input: `{"field": "name", "operator": "like", "value": "Alice"}`,
			expected: Condition{
				Field:    "name",
				Operator: OpLike,
				Value:    "Alice",
			},
			wantErr: false,
		},
		{
			name:  "array value for IN operator",
			input: `{"field": "id", "operator": "in", "value": [1, 2, 3]}`,
			expected: Condition{
				Field:    "id",
				Operator: OpIn,
				Value:    []interface{}{float64(1), float64(2), float64(3)},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cond Condition
			err := json.Unmarshal([]byte(tt.input), &cond)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Field, cond.Field)
			assert.Equal(t, tt.expected.Operator, cond.Operator)
			assert.Equal(t, tt.expected.Value, cond.Value)
		})
	}
}

func TestWhereDTO_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *WhereDTO
		wantErr  bool
	}{
		{
			name:  "simple flat conditions",
			input: `{"type": "and", "conditions": [{"field": "status", "operator": "eq", "value": 1}]}`,
			expected: &WhereDTO{
				Type: "and",
				Conditions: []ConditionOrGroup{
					{Condition: &Condition{Field: "status", Operator: OpEq, Value: float64(1)}},
				},
			},
			wantErr: false,
		},
		{
			name:  "nested OR group",
			input: `{"type": "and", "conditions": [{"field": "status", "operator": "eq", "value": 1}, {"type": "or", "conditions": [{"field": "name", "operator": "like", "value": "Alice"}]}]}`,
			expected: &WhereDTO{
				Type: "and",
				Conditions: []ConditionOrGroup{
					{Condition: &Condition{Field: "status", Operator: OpEq, Value: float64(1)}},
					{
						Group: &WhereDTO{
							Type: "or",
							Conditions: []ConditionOrGroup{
								{Condition: &Condition{Field: "name", Operator: OpLike, Value: "Alice"}},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dto WhereDTO
			err := json.Unmarshal([]byte(tt.input), &dto)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Type, dto.Type)
			assert.Equal(t, len(tt.expected.Conditions), len(dto.Conditions))
		})
	}
}

func TestQueryDTO_UnmarshalJSON(t *testing.T) {
	input := `{
		"where": {
			"type": "and",
			"conditions": [
				{"field": "status", "operator": "eq", "value": 1},
				{
					"type": "or",
					"conditions": [
						{"field": "name", "operator": "like", "value": "Alice"},
						{"field": "email", "operator": "like", "value": "alice@example.com"}
					]
				}
			]
		},
		"order": [
			{"field": "create_time", "order": "desc"},
			{"field": "update_time", "order": "asc"}
		]
	}`

	var dto QueryDTO
	err := json.Unmarshal([]byte(input), &dto)
	require.NoError(t, err)

	// 验证 Where
	require.NotNil(t, dto.Where)
	assert.Equal(t, "and", dto.Where.Type)
	require.GreaterOrEqual(t, len(dto.Where.Conditions), 2)

	// 第一个条件应该是 status = 1
	cond0 := dto.Where.Conditions[0]
	require.NotNil(t, cond0.Condition)
	assert.Equal(t, "status", cond0.Condition.Field)
	assert.Equal(t, OpEq, cond0.Condition.Operator)

	// 第二个条件应该是嵌套的 OR 组
	cond1 := dto.Where.Conditions[1]
	require.NotNil(t, cond1.Group, "nested group should be in Group field")
	assert.Equal(t, "or", cond1.Group.Type)
	require.Len(t, cond1.Group.Conditions, 2)

	// 验证嵌套组内的条件
	nestedCond0 := cond1.Group.Conditions[0]
	require.NotNil(t, nestedCond0.Condition)
	assert.Equal(t, "name", nestedCond0.Condition.Field)
	assert.Equal(t, OpLike, nestedCond0.Condition.Operator)

	// 验证 Order
	require.Len(t, dto.Order, 2)
	assert.Equal(t, "create_time", dto.Order[0].Field)
	assert.Equal(t, OrderDesc, dto.Order[0].Order)
	assert.Equal(t, "update_time", dto.Order[1].Field)
	assert.Equal(t, OrderAsc, dto.Order[1].Order)
}

func TestSortOrder_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SortOrder
	}{
		{"asc", `"asc"`, OrderAsc},
		{"desc", `"desc"`, OrderDesc},
		// 注意：UnmarshalJSON 不会自动转换大小写，保持原始值
		// BuildOrder 方法会处理大小写转换
		{"ASC uppercase", `"ASC"`, SortOrder("ASC")},
		{"DESC uppercase", `"DESC"`, SortOrder("DESC")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order SortOrder
			err := json.Unmarshal([]byte(tt.input), &order)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, order)
		})
	}
}

func TestOperator_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Operator
	}{
		{"eq", `"eq"`, OpEq},
		{"ne", `"ne"`, OpNe},
		{"gt", `"gt"`, OpGt},
		{"gte", `"gte"`, OpGte},
		{"lt", `"lt"`, OpLt},
		{"lte", `"lte"`, OpLte},
		{"like", `"like"`, OpLike},
		{"in", `"in"`, OpIn},
		{"is_null", `"is_null"`, OpIsNull},
		{"is_not_null", `"is_not_null"`, OpIsNotNull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var op Operator
			err := json.Unmarshal([]byte(tt.input), &op)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, op)
		})
	}
}
