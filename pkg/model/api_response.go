package model

// ApiResponse 统一API响应格式（匹配前端 ApiResponse.ts）
type ApiResponse struct {
	Success bool        `json:"success"` // 是否成功
	Msg     string      `json:"msg"`     // 消息
	Data    interface{} `json:"data"`    // 数据
}

// Success 成功响应
func Success(data interface{}) *ApiResponse {
	return &ApiResponse{
		Success: true,
		Msg:     "success",
		Data:    data,
	}
}

// Error 错误响应
func Error(message string) *ApiResponse {
	return &ApiResponse{
		Success: false,
		Msg:     message,
		Data:    nil,
	}
}

// ErrorWithCode 带错误码的响应
func ErrorWithCode(code int, message string) *ApiResponse {
	return &ApiResponse{
		Success: code == 0,
		Msg:     message,
		Data:    nil,
	}
}
