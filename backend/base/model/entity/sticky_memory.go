package entity

import "github.com/library-squirrel/backend/base/model"

// 粘性记忆域取值（sticky_memory.domain 列的封闭取值域，一域 = 一张交互冲突面）。
// 语义统一为「该站点域内这对候选里用户显选用过谁」；域值入列前由写入方取下列常量，
// 禁止裸串散落（域歧义会令两张面的记忆互串）
const (
	// DomainTaskURLDisambiguation 任务 URL 创建时多插件候选冲突的显选记忆域
	DomainTaskURLDisambiguation = "task_url_disambiguation"
	// DomainSiteAuthorFetchDisambiguation 站点作者手动拉取时多插件候选冲突的显选记忆域
	DomainSiteAuthorFetchDisambiguation = "site_author_fetch_disambiguation"
)

// StickyMemory 粘性记忆行：用户在交互冲突面（多插件候选弹窗）的显选记录。同域同键的
// 冲突复现时按记忆直接路由不再询问。(domain, context_key) 唯一——一行表达一个
// 「站点域 × 候选组合」对的当前选择，重选覆盖 value 不另起行。程序化/自动入口不读写本表
type StickyMemory struct {
	*model.BaseEntity
	// Domain 记忆域（封闭取值域，见 Domain* 常量），区分是哪张冲突面记下的选择
	Domain string `gorm:"column:domain;index:idx_sticky_memory_domain;uniqueIndex:idx_sticky_memory_domain_context_key;not null" json:"domain"`
	// ContextKey 记忆上下文键 = 站点域与候选组合的编码串。不透明键：构造与拆解统一走
	// stickymemory 包的编码函数，其余代码禁止手拼
	ContextKey string `gorm:"column:context_key;uniqueIndex:idx_sticky_memory_domain_context_key;not null" json:"contextKey"`
	// Value 显选候选全键（与 ContextKey 内候选全键同形：插件 PublicID + NUL + 扩展点 ID）
	Value string `gorm:"column:value;not null" json:"value"`
}

func (StickyMemory) TableName() string {
	return "sticky_memory"
}

// NewStickyMemory 创建粘性记忆行（工厂方法，禁止 &StickyMemory{}）
func NewStickyMemory() *StickyMemory {
	return &StickyMemory{BaseEntity: &model.BaseEntity{}}
}
