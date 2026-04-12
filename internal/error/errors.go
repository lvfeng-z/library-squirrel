package error

import (
	"errors"
	"fmt"
)

// 业务错误码定义
const (
	CodeSuccess       = 0
	CodeBadRequest    = 400
	CodeUnauthorized  = 401
	CodeForbidden     = 403
	CodeNotFound      = 404
	CodeInternalError = 500
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *BusinessError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *BusinessError) Unwrap() error {
	return e.Err
}

// NewBusinessError 创建业务错误
func NewBusinessError(code int, message string) *BusinessError {
	return &BusinessError{Code: code, Message: message}
}

// NewBusinessErrorWithError 创建带底层错误的业务错误
func NewBusinessErrorWithError(code int, message string, err error) *BusinessError {
	return &BusinessError{Code: code, Message: message, Err: err}
}

// 常用错误构造函数
func BadRequest(message string) *BusinessError {
	return NewBusinessError(CodeBadRequest, message)
}

func NotFound(message string) *BusinessError {
	return NewBusinessError(CodeNotFound, message)
}

func InternalError(message string) *BusinessError {
	return NewBusinessError(CodeInternalError, message)
}

func InternalErrorWithError(message string, err error) *BusinessError {
	return NewBusinessErrorWithError(CodeInternalError, message, err)
}

// IsBusinessError 判断是否为业务错误
func IsBusinessError(err error) bool {
	var be *BusinessError
	return errors.As(err, &be)
}

// GetBusinessError 获取业务错误
func GetBusinessError(err error) *BusinessError {
	var be *BusinessError
	if errors.As(err, &be) {
		return be
	}
	return nil
}
