package entity

import "github.com/library-squirrel/backend/base/model"

// ExternalApiOpRecord 外部操作记录行：外部 AI 操作接口调用的事后问责账——每次经操作
// 分发层的外部调用落一行（操作标识、脱敏参数摘要、成败、耗时、调用方自报标签）。它是
// 被动记录，不参与任何事前把关。行只追加不更新；id 自增即消费游标（全表单调递增），
// 老化行由 externalapi 模块按 TTL 与容量轮转裁最旧（物理 DELETE），无软删语义、无外键
type ExternalApiOpRecord struct {
	*model.BaseEntity
	// Operation 操作标识（统一操作端点 URL 的路径段，如 task.createByURL）
	Operation string `gorm:"column:operation;not null" json:"operation"`
	// Params 脱敏参数摘要（请求体的紧凑 JSON；URL 类值仅落协议+主机+路径，查询串只记存在
	// 与字符数，防凭据类查询参数进入记录与设置页展示；超长截断，空串 = 无参数）
	Params string `gorm:"column:params" json:"params"`
	// Success 本次调用是否以成功信封收口（失败行供问责定位失败原因）
	Success bool `gorm:"column:success;not null" json:"success"`
	// ClientLabel 调用方自报的客户端标签（请求头携带，空 = 未自报）。自报归属、非鉴别凭据
	ClientLabel string `gorm:"column:client_label" json:"clientLabel"`
	// DurationMs 本次调用耗时毫秒
	DurationMs int64 `gorm:"column:duration_ms" json:"durationMs"`
}

func (ExternalApiOpRecord) TableName() string {
	return "external_api_op_record"
}

// NewExternalApiOpRecord 创建外部操作记录行（工厂方法，禁止 &ExternalApiOpRecord{}）
func NewExternalApiOpRecord() *ExternalApiOpRecord {
	return &ExternalApiOpRecord{BaseEntity: &model.BaseEntity{}}
}
