package model

import (
	"github.com/library-squirrel/backend/base/logger"
)

// ApiResponse 统一API响应格式（匹配前端 ApiResponse.ts）。
// 语义单向不变量：Success 为 false ⇔ 纯失败——操作未达成，Msg 为用户可读原因，Data 无意义（恒零值，
// 只经 Error/HandleError/HandleResult 构造，禁止手工构造携带 Data 的失败响应）；一切非失败的业务
// 分支态（候选冲突待用户显选、批量逐条结果、降级成功）住 Success 为 true 的 Data 载荷内，由载荷
// 自带标志表达。Msg 在成功时为附注说明（默认 "success"，降级/引导文案经 SuccessWithMsg），不承载
// 调用方的分支判定
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

// SuccessWithMsg 成功响应（自定义 Msg）：操作主体成功但需向用户附带降级说明时使用
// （如插件已安装/已信任但激活失败），前端成功分支按 Msg 与默认文案 "success" 的差异提示降级
func SuccessWithMsg[T any](data T, msg string) *ApiResponse[T] {
	return &ApiResponse[T]{
		Success: true,
		Msg:     msg,
		Data:    data,
	}
}

// Error 错误响应（纯失败：Data 恒零值，与 ApiResponse 的单向不变量一致）
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
