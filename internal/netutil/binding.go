package netutil

//
//import (
//	"encoding/json"
//	"errors"
//	"io"
//	"net/http"
//	"reflect"
//	"strings"
//)
//
//// ErrJSONEmptyBody JSON body 为空的错误
//var ErrJSONEmptyBody = errors.New("json body is empty")
//
//// CustomJSONBinding 自定义 JSON 绑定器
//// 在反序列化时自动初始化嵌入的 *BaseEntity 指针
//type CustomJSONBinding struct {
//	StrictMode bool
//}
//
//// Bind 实现自定义绑定
//func (b *CustomJSONBinding) Bind(req *http.Request, obj interface{}) error {
//	// 读取请求体
//	body, err := readBody(req)
//	if err != nil {
//		return err
//	}
//
//	// 先进行标准的 JSON 反序列化
//	if err := json.Unmarshal(body, obj); err != nil {
//		return err
//	}
//
//	// 自动初始化嵌入的 BaseEntity 指针
//	InitEmbeddedBaseEntity(obj)
//
//	return nil
//}
//
//// Name 返回绑定器名称
//func (b *CustomJSONBinding) Name() string {
//	return "customjson"
//}
//
//// readBody 读取请求体
//func readBody(req *http.Request) ([]byte, error) {
//	if req.Body == nil {
//		return nil, ErrJSONEmptyBody
//	}
//	body, err := io.ReadAll(req.Body)
//	if err != nil {
//		return nil, err
//	}
//	// 重置 Body 以便后续处理可以再次读取
//	req.Body = io.NopCloser(strings.NewReader(string(body)))
//	return body, nil
//}
//
//// InitEmbeddedBaseEntity 检查并初始化嵌入的 *BaseEntity 指针
//// JSON 反序列化时嵌入的指针可能为 nil，需要在访问前初始化
//func InitEmbeddedBaseEntity(entity interface{}) {
//	v := reflect.ValueOf(entity).Elem()
//	baseField := v.FieldByName("BaseEntity")
//	if baseField.IsValid() && baseField.Kind() == reflect.Ptr && baseField.IsNil() {
//		baseField.Set(reflect.New(baseField.Type().Elem()))
//	}
//}
