package model

// ApiResponse 统一API响应格式（匹配前端 ApiResponse.ts）
type ApiResponse[T any] struct {
	Success bool   `json:"success"` // 是否成功
	Msg     string `json:"msg"`     // 消息
	Data    T      `json:"data"`    // 数据
}

// Success 成功响应
func Success[T any](data T) *ApiResponse[T] {
	return &ApiResponse[T]{
		Success: true,
		Msg:     "success",
		Data:    data,
	}
}

// Error 错误响应
func Error[T any](message string) *ApiResponse[T] {
	var zero T
	return &ApiResponse[T]{
		Success: false,
		Msg:     message,
		Data:    zero,
	}
}
