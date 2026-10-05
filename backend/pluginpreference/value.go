package pluginpreference

import "encoding/json"

// PreferenceValue 偏好值信封：plugin_preference.value 列 JSON 文本的 Go 形态。
// title/description 由插件写入时随值携带，供管理面展示决策内容（区别于清单静态声明，
// 决策标题在问答发生时才确定）；schemaVersion 标记值结构版本（规则类值会演进，无版本
// 的结构变更会静默损坏既有记忆）；data 为插件自定义负载，宿主只存不解释
type PreferenceValue struct {
	SchemaVersion int    `json:"schemaVersion"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Data          any    `json:"data"`
}

// marshalValue 信封 → value 列 JSON 文本（整值序列化，落库即完整覆写单位）
func marshalValue(v *PreferenceValue) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// parseValue value 列 JSON 文本 → 信封。值列由本模块 marshal 写入，非法文本属异常
// 数据，返回错误由调用方决定处置（读取面报错、管理列表按空信封降级）
func parseValue(raw string) (*PreferenceValue, error) {
	var v PreferenceValue
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, err
	}
	return &v, nil
}
