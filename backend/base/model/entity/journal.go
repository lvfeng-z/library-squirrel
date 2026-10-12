package entity

import "github.com/library-squirrel/backend/base/model"

// 记账域取值（journal_entry.domain 列的封闭取值域，一域 = 一类记账目录）。
// 域值入列前由写入方取下列常量，禁止裸串散落（域歧义会令不同目录的账行互串）
const (
	// JournalDomainTask 任务域：任务终态迁移记账（主体引用 = 任务 ID）
	JournalDomainTask = "task"
	// JournalDomainIngest 入库域：作品入库完成记账（主体引用 = 作品 ID）
	JournalDomainIngest = "ingest"
)

// JournalEntry 领域记账行：一张 append-only 账表统一承载各领域事件账，供消费方按游标
// 读取变化史。行只追加不更新；id 即消费游标（自增、全表单调递增），跨域共用一条游标流，
// 不按域分表。老化行由 journal 模块轮转裁最旧（物理 DELETE），无软删语义、无外键
type JournalEntry struct {
	*model.BaseEntity
	// Domain 记账域（封闭取值域，见 JournalDomain* 常量）：账行归属的记账目录
	Domain string `gorm:"column:domain;not null" json:"domain"`
	// EventType 事件类型（域内事件分类，取值由各域的记账适配方定义，如任务终态迁移、作品入库完成）
	EventType string `gorm:"column:event_type;not null" json:"eventType"`
	// SubjectID 主体引用：域内主键（任务域 = 任务 ID、入库域 = 作品 ID），消费方据此回查主体行
	SubjectID int64 `gorm:"column:subject_id;not null" json:"subjectId"`
	// Payload 摘要载荷（JSON 文本，如任务终态值、作品标题；空串 = 无摘要）
	Payload string `gorm:"column:payload" json:"payload"`
}

func (JournalEntry) TableName() string {
	return "journal_entry"
}

// NewJournalEntry 创建记账行（工厂方法，禁止 &JournalEntry{}）
func NewJournalEntry() *JournalEntry {
	return &JournalEntry{BaseEntity: &model.BaseEntity{}}
}
