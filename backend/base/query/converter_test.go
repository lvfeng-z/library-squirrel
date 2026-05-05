package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

// 测试用的 TaskQueryDTO
type testTaskQueryDTO struct {
	TaskName   QueryAttribute `json:"taskName" query:"task_name"`
	SiteId     QueryAttribute `json:"siteId" query:"site_id"`
	Status     QueryAttribute `json:"status" query:"status"`
	CreateTime QueryAttribute `json:"createTime" query:"create_time"`
	// 非数据库字段
	ExtraField QueryAttribute `json:"extraField" query:""`
}

func TestConverter_ToQueryOption_Basic(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName: QueryAttribute{Value: "test", Operator: OpLike},
		SiteId:   QueryAttribute{Value: 123},
		Status:   QueryAttribute{Value: 1},
	}

	result, err := conv.ToQueryOption(dto)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 验证有条件
	require.NotEmpty(t, result.Conditions)
}

func TestConverter_ToQueryOption_IgnoreNonDbField(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName:   QueryAttribute{Value: "test"},
		ExtraField: QueryAttribute{Value: "should be ignored"},
	}

	result, err := conv.ToQueryOption(dto)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 验证 ExtraField 被跳过
	require.Len(t, result.Conditions, 1)
}

func TestConverter_ToQueryOption_WithOrder(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		TaskName:   QueryAttribute{Value: "test", Operator: OpLike, Order: OrderAsc},
		Status:     QueryAttribute{Value: 1, Order: OrderDesc},
		CreateTime: QueryAttribute{Order: OrderAsc}, // 只有排序，没有值
	}

	result, err := conv.ToQueryOption(dto)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 验证有排序
	require.NotEmpty(t, result.OrderBy)
	orderBy := result.OrderBy[0].(clause.OrderBy)
	require.Len(t, orderBy.Columns, 3) // TaskName, Status, CreateTime
}

func TestConverter_ToQueryOption_NilValueWithOrder(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		CreateTime: QueryAttribute{Order: OrderAsc}, // 只有排序，没有值
	}

	result, err := conv.ToQueryOption(dto)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 验证排序存在但无条件
	require.Empty(t, result.Conditions)
	require.NotEmpty(t, result.OrderBy)
}

func TestConverter_ToQueryOption_AllOperators(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	testCases := []struct {
		name     string
		attr     QueryAttribute
		expected bool
	}{
		{"eq", QueryAttribute{Value: 1, Operator: OpEq}, true},
		{"ne", QueryAttribute{Value: 1, Operator: OpNe}, true},
		{"gt", QueryAttribute{Value: 1, Operator: OpGt}, true},
		{"gte", QueryAttribute{Value: 1, Operator: OpGte}, true},
		{"lt", QueryAttribute{Value: 1, Operator: OpLt}, true},
		{"lte", QueryAttribute{Value: 1, Operator: OpLte}, true},
		{"like", QueryAttribute{Value: "test", Operator: OpLike}, true},
		{"in", QueryAttribute{Value: []interface{}{1, 2, 3}, Operator: OpIn}, true},
		{"is_null", QueryAttribute{Operator: OpIsNull}, true},
		{"is_not_null", QueryAttribute{Operator: OpIsNotNull}, true},
		{"default_eq", QueryAttribute{Value: 1}, true}, // 默认 eq
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dto := testTaskQueryDTO{
				TaskName: tc.attr,
			}

			result, err := conv.ToQueryOption(dto)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Conditions)
		})
	}
}

func TestConverter_ToPageOption(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{
		Status: QueryAttribute{Value: 1, Order: OrderDesc},
	}

	result, err := conv.ToPageOption(dto, 1, 10)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 10, result.PageSize)
}

func TestConverter_ToQueryOption_EmptyDto(t *testing.T) {
	conv := NewConverter(testTaskQueryDTO{})

	dto := testTaskQueryDTO{}

	result, err := conv.ToQueryOption(dto)
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
		attr     QueryAttribute
		expected Operator
	}{
		{"explicit_eq", QueryAttribute{Value: 1, Operator: OpEq}, OpEq},
		{"explicit_like", QueryAttribute{Value: "test", Operator: OpLike}, OpLike},
		{"empty", QueryAttribute{Value: 1}, OpEq}, // 默认 eq
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
		attr     QueryAttribute
		expected bool
	}{
		{"with_order", QueryAttribute{Order: OrderAsc}, true},
		{"empty_order", QueryAttribute{Value: 1}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.attr.HasOrder())
		})
	}
}
