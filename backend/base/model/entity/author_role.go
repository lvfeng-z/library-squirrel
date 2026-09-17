package entity

import (
	"github.com/library-squirrel/backend/base/model"
)

// AuthorRole 作者 role 维度清单行（re_work_author 关联级 role 维度的候选值登记）。
// 清单 = 「见过的值」而非约束：re_work_author.role_name 为开放字符串，未知值允许写入不依赖本表
type AuthorRole struct {
	*model.BaseEntity
	// Value 归一化后的维度值（去首尾空白 + 小写折叠），清单唯一键
	Value string `gorm:"column:value;uniqueIndex:idx_author_role_value;not null" json:"value"`
	// Label 显示名，空串=前端直接展示 value
	Label string `gorm:"column:label" json:"label"`
	// Origin 来源三态（constant.ORIGIN_PLUGIN/USER/BUILTIN；零值=plugin——缺省来源取最低优先级）。
	// 优先级 builtin > user > plugin：同值多来源时取最高优先级，内置行的 label/origin 为权威值不被改写
	Origin int64 `gorm:"column:origin;not null;default:0" json:"origin"`
	// LastUse 最近使用毫秒时间戳，0=从未使用（使用统计属展示排序，不参与来源权威）
	LastUse int64 `gorm:"column:last_use" json:"lastUse"`
}

func (AuthorRole) TableName() string {
	return "author_role"
}

func NewAuthorRole() *AuthorRole {
	return &AuthorRole{
		BaseEntity: &model.BaseEntity{},
	}
}
