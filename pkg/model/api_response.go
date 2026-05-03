package model

import (
	"github.com/library-squirrel/wails/pkg/logger"
)

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

// HandleResult 统一处理带返回值的结果，记录错误日志并返回 ApiResponse
func HandleResult[T any](result T, err error) *ApiResponse[T] {
	if err != nil {
		logger.Log.Errorf("[Handler] %v", err)
		return Error[T](err.Error())
	}
	return Success(result)
}

// HandleVoid 统一处理无返回值的结果，记录错误日志并返回 ApiResponse
func HandleVoid(err error) *ApiResponse[any] {
	return HandleResult[any](nil, err)
}

// HandleError 记录错误日志并返回错误 ApiResponse（用于无法使用 HandleResult 的场景，如 DTO 转换后调用）
func HandleError[T any](err error) *ApiResponse[T] {
	logger.Log.Errorf("[Handler] %v", err)
	return Error[T](err.Error())
}
