package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// ptr 返回值指针(测试构造 QueryAttribute[T] 字面量用;字段均为指针类型)
func ptr[T any](v T) *T { return &v }

// 测试用的 TaskQueryDTO(字段类型参照 task.TaskQueryDTO 的泛型用法)
type testTaskQueryDTO struct {
	TaskName   QueryAttribute[string] `json:"taskName" query:"task_name"`
	SiteId     QueryAttribute[int64]  `json:"siteId" query:"site_id"`
	Status     QueryAttribute[int64]  `json:"status" query:"status"`
	CreateTime QueryAttribute[int64]  `json:"createTime" query:"create_time"`
	// 非数据库字段
	ExtraField QueryAttribute[string] `json:"extraField" query:""`
}

// testAnyDTO 用于操作符覆盖测试:Value 为 any,可承载 OpIn 的 []interface{} 等任意类型
type testAnyDTO struct {
	Field QueryAttribute[any] `query:"field"`
}

func TestConverter_ToQueryOption_Basic(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName: QueryAttribute[string]{Value: ptr("test"), Operator: ptr(OpLike)},
		SiteId:   QueryAttribute[int64]{Value: ptr(int64(123))},
		Status:   QueryAttribute[int64]{Value: ptr(int64(1))},
	}

	result, err := conv.ToQueryOption(dto, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, result.Conditions)
}

func TestConverter_ToQueryOption_IgnoreNonDbField(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName:   QueryAttribute[string]{Value: ptr("test")},
		ExtraField: QueryAttribute[string]{Value: ptr("should be ignored")},
	}

	result, err := conv.ToQueryOption(dto, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Conditions, 1)
}

func TestConverter_ToQueryOption_WithOrder(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName:   QueryAttribute[string]{Value: ptr("test"), Operator: ptr(OpLike), Order: ptr(OrderAsc)},
		Status:     QueryAttribute[int64]{Value: ptr(int64(1)), Order: ptr(OrderDesc)},
		CreateTime: QueryAttribute[int64]{Order: ptr(OrderAsc)}, // 只有排序，没有值
	}

	result, err := conv.ToQueryOption(dto, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, result.OrderBy)
	orderBy := result.OrderBy[0].(clause.OrderBy)
	require.Len(t, orderBy.Columns, 3) // TaskName, Status, CreateTime
}

func TestConverter_ToQueryOption_NilValueWithOrder(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		CreateTime: QueryAttribute[int64]{Order: ptr(OrderAsc)}, // 只有排序，没有值
	}

	result, err := conv.ToQueryOption(dto, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Empty(t, result.Conditions)
	require.NotEmpty(t, result.OrderBy)
}

func TestConverter_ToQueryOption_AllOperators(t *testing.T) {
	conv := NewConverter(testAnyDTO{})

	testCases := []struct {
		name string
		attr QueryAttribute[any]
	}{
		{"eq", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpEq)}},
		{"ne", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpNe)}},
		{"gt", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpGt)}},
		{"gte", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpGte)}},
		{"lt", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpLt)}},
		{"lte", QueryAttribute[any]{Value: ptr(any(1)), Operator: ptr(OpLte)}},
		{"like", QueryAttribute[any]{Value: ptr(any("test")), Operator: ptr(OpLike)}},
		{"in", QueryAttribute[any]{Value: ptr(any([]interface{}{1, 2, 3})), Operator: ptr(OpIn)}},
		{"is_null", QueryAttribute[any]{Operator: ptr(OpIsNull)}},
		{"is_not_null", QueryAttribute[any]{Operator: ptr(OpIsNotNull)}},
		{"default_eq", QueryAttribute[any]{Value: ptr(any(1))}}, // 默认 eq
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dto := testAnyDTO{Field: tc.attr}

			result, err := conv.ToQueryOption(dto, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Conditions)
		})
	}
}

func TestConverter_ToPageOption(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		Status: QueryAttribute[int64]{Value: ptr(int64(1)), Order: ptr(OrderDesc)},
	}

	result, err := conv.ToPageOption(dto, 1, 10, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 10, result.PageSize)
}

func TestConverter_ToQueryOption_EmptyDto(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{}

	result, err := conv.ToQueryOption(dto, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	// 空 DTO 应该返回空的 QueryOption
}

func TestFieldMapper_GetColumnName(t *testing.T) {
	mapper := NewFieldMapper(testTaskQueryDTO{})

	testCases := []struct {
		fieldName    string
		expectedCol  string
		expectedIsDb bool
	}{
		{"TaskName", "task_name", true},
		{"SiteId", "site_id", true},
		{"Status", "status", true},
		{"ExtraField", "", false}, // query tag 为空
		{"NonExistent", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.fieldName, func(t *testing.T) {
			col, isDb := mapper.GetColumnName(tc.fieldName)
			assert.Equal(t, tc.expectedCol, col)
			assert.Equal(t, tc.expectedIsDb, isDb)
		})
	}
}

func TestQueryAttribute_GetOperator(t *testing.T) {
	testCases := []struct {
		name     string
		attr     QueryAttribute[int]
		expected Operator
	}{
		{"explicit_eq", QueryAttribute[int]{Value: ptr(1), Operator: ptr(OpEq)}, OpEq},
		{"explicit_like", QueryAttribute[int]{Value: ptr(1), Operator: ptr(OpLike)}, OpLike},
		{"empty", QueryAttribute[int]{Value: ptr(1)}, OpEq}, // 默认 eq
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.attr.GetOperator())
		})
	}
}

func TestQueryAttribute_HasOrder(t *testing.T) {
	testCases := []struct {
		name     string
		attr     QueryAttribute[int]
		expected bool
	}{
		{"with_order", QueryAttribute[int]{Order: ptr(OrderAsc)}, true},
		{"empty_order", QueryAttribute[int]{Value: ptr(1)}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.attr.HasOrder())
		})
	}
}
