package query

import (
	"fmt"
	"reflect"
)

// FieldMapper 通过反射获取结构体字段的 query tag 映射
type FieldMapper struct {
	structType reflect.Type
}

// NewFieldMapper 创建字段映射器
func NewFieldMapper(model interface{}) *FieldMapper {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return &FieldMapper{structType: t}
}

// GetColumnName 获取 query tag 映射的列名
// 返回 (列名, 是否为数据库字段)
// 如果 query tag 为空，返回 (空字符串, false) 表示非数据库字段
func (m *FieldMapper) GetColumnName(fieldName string) (string, bool) {
	field, ok := m.structType.FieldByName(fieldName)
	if !ok {
		return "", false
	}
	colName := field.Tag.Get("query")
	if colName == "" {
		return "", false // query tag 为空，非数据库字段
	}
	return colName, true
}

// IterateFields 遍历结构体所有字段，对每个字段执行fn
// fn 接收 (字段名, query列名, 是否为数据库字段)
// 如果 fn 返回 false，停止遍历
func (m *FieldMapper) IterateFields(fn func(fieldName, columnName string, isDbField bool) bool) {
	for i := 0; i < m.structType.NumField(); i++ {
		field := m.structType.Field(i)
		fieldName := field.Name
		colName := field.Tag.Get("query")
		isDbField := colName != ""
		if !fn(fieldName, colName, isDbField) {
			return
		}
	}
}

// FieldInfo 字段信息
type FieldInfo struct {
	FieldName  string               // 结构体字段名
	ColumnName string               // query tag 映射的列名
	IsDbField  bool                 // 是否为数据库字段
	QueryAttr  *QueryAttribute[any] // 查询属性
}

// CollectDbFields 收集所有数据库字段的查询信息
func (m *FieldMapper) CollectDbFields(dto interface{}) ([]FieldInfo, error) {
	var fields []FieldInfo

	v := reflect.ValueOf(dto)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("dto must be a struct, got %v", v.Kind())
	}

	for i := 0; i < v.NumField(); i++ {
		fieldType := v.Type().Field(i)
		fieldValue := v.Field(i)

		fieldName := fieldType.Name
		colName := fieldType.Tag.Get("query")
		isDbField := colName != ""

		// 获取 QueryAttribute 值
		var attr *QueryAttribute[any]
		if fieldValue.CanInterface() {
			if attrVal, ok := fieldValue.Interface().(QueryAttribute[any]); ok {
				attr = &attrVal
			}
		}

		fields = append(fields, FieldInfo{
			FieldName:  fieldName,
			ColumnName: colName,
			IsDbField:  isDbField,
			QueryAttr:  attr,
		})
	}

	return fields, nil
}
