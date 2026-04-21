package util

// ToAnySlice 切片转换为any切片
func ToAnySlice[T any](slice []T) []any {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}
