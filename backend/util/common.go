package util

import "database/sql"

// ToAnySlice 切片转换为any切片
func ToAnySlice[T any](slice []T) []any {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

// NullStringToPointer 将 sql.NullString 转换为 *string
func NullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// NullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func NullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}

// StringPtrIfValid 将 string 转换为 *string（非空时返回指针）
func StringPtrIfValid(s string) *string {
	if s != "" {
		return &s
	}
	return nil
}

// Int64PtrIfValid 将 int64 转换为 *int64（非零时返回指针）
func Int64PtrIfValid(i int64) *int64 {
	if i != 0 {
		return &i
	}
	return nil
}

// UniqueInt64 对Int64的切片进行去重
func UniqueInt64(ids []int64) []int64 {
	seen := make(map[int64]struct{})
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

// UniqueString 对string的切片进行去重
func UniqueString(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
