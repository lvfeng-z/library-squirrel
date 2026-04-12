package netutil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrJSONEmptyBody JSON body 为空的错误
var ErrJSONEmptyBody = errors.New("json body is empty")

// CustomJSONBinding 自定义 JSON 绑定器
// 在反序列化时自动初始化嵌入的 *BaseEntity 指针
type CustomJSONBinding struct {
	StrictMode bool
}

// Bind 实现自定义绑定
func (b *CustomJSONBinding) Bind(req *http.Request, obj interface{}) error {
	// 读取请求体
	body, err := readBody(req)
	if err != nil {
		return err
	}

	// 先进行标准的 JSON 反序列化
	if err := json.Unmarshal(body, obj); err != nil {
		return err
	}

	// 自动初始化嵌入的 BaseEntity 指针
	InitEmbeddedBaseEntity(obj)

	return nil
}

// Name 返回绑定器名称
func (b *CustomJSONBinding) Name() string {
	return "customjson"
}

// readBody 读取请求体
func readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, ErrJSONEmptyBody
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	// 重置 Body 以便后续处理可以再次读取
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, nil
}

// InitEmbeddedBaseEntity 检查并初始化嵌入的 *BaseEntity 指针
// JSON 反序列化时嵌入的指针可能为 nil，需要在访问前初始化
func InitEmbeddedBaseEntity(entity interface{}) {
	v := reflect.ValueOf(entity).Elem()
	baseField := v.FieldByName("BaseEntity")
	if baseField.IsValid() && baseField.Kind() == reflect.Ptr && baseField.IsNil() {
		baseField.Set(reflect.New(baseField.Type().Elem()))
	}
}

// AutoBindHandler 是一个通用的自动绑定包装器
// 支持两种 handler 签名：
//  1. func(c *gin.Context, req *T) error
//  2. func(c *gin.Context, req *T) (interface{}, error)
func AutoBindHandler(handler interface{}) gin.HandlerFunc {
	// 1. 获取 handler 的反射值
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()

	// 2. 校验 handler 的签名
	if handlerType.Kind() != reflect.Func {
		panic("handler must be a function")
	}
	// 获取 *gin.Context 类型进行对比
	ginContextPtrType := reflect.TypeOf((*gin.Context)(nil))
	if handlerType.NumIn() < 1 || handlerType.In(0) != ginContextPtrType {
		panic("handler must have *gin.Context as the first parameter")
	}

	// 检测返回值类型：支持 (interface{}, error) 或只返回 error
	returnsData := handlerType.NumOut() == 2 &&
		handlerType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() &&
		handlerType.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem())

	if !returnsData && (handlerType.NumOut() != 1 || !handlerType.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem())) {
		panic("handler must return (interface{}, error) or just error")
	}

	// 3. 返回 Gin 处理器
	return func(c *gin.Context) {
		// 准备调用参数
		// 第一个参数永远是 *gin.Context
		callParams := []reflect.Value{reflect.ValueOf(c)}

		// 4. 遍历 handler 的其他参数，进行自动绑定
		for i := 1; i < handlerType.NumIn(); i++ {
			paramType := handlerType.In(i)

			// 检查参数是否是指针类型
			if paramType.Kind() != reflect.Ptr {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "parameter must be a pointer"})
				return
			}

			// 创建一个新的参数实例
			newInstance := reflect.New(paramType.Elem())

			// 5. 执行自定义绑定（包含 BaseEntity 初始化）
			customBinding := CustomJSONBinding{StrictMode: true}
			if err := customBinding.Bind(c.Request, newInstance.Interface()); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// 将绑定好的实例添加到调用参数列表中
			callParams = append(callParams, newInstance)
		}

		// 6. 调用原始业务方法
		results := handlerValue.Call(callParams)

		// 7. 处理返回值
		if returnsData {
			// 模式 2: (interface{}, error)
			if err, _ := results[1].Interface().(error); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// 成功返回数据
			c.JSON(http.StatusOK, results[0].Interface())
		} else {
			// 模式 1: 只有 error
			if err, _ := results[0].Interface().(error); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// 成功返回
			c.JSON(http.StatusOK, gin.H{"success": true, "msg": "success"})
		}
	}
}

// parseJSONBody 解析 JSON 请求体到指定类型
// 用于在没有 AutoBindHandler 的情况下手动处理
func ParseJSONBody(c *gin.Context, obj interface{}) error {
	body, err := readBody(c.Request)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, obj); err != nil {
		return err
	}
	InitEmbeddedBaseEntity(obj)
	return nil
}

// MustParseJSONBody 类似 ParseJSONBody，但会在失败时 abort
func MustParseJSONBody(c *gin.Context, obj interface{}) {
	if err := ParseJSONBody(c, obj); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
}

// GetRequestJSON 快捷方法：从请求中解析 JSON 到结构体
// 示例: var req TagDTO; if err := GetRequestJSON(c, &req); err != nil { ... }
func GetRequestJSON(c *gin.Context, obj interface{}) error {
	return ParseJSONBody(c, obj)
}

// ========== 辅助方法 ==========

// GetBodyFromRequest 从请求中获取 body 字符串（用于调试）
func GetBodyFromRequest(c *gin.Context) string {
	body, _ := readBody(c.Request)
	return strings.TrimSpace(string(body))
}
