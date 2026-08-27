package entity

import (
	"github.com/library-squirrel/backend/base/model"
)

// ShareRecord 分享记录行（分享方持久化账本）。会话运行态（connecting/online/reconnecting）
// 不入表——运行期由进程内会话注册表实时承载；本表记录首次在线起的分享参数与终态，
// 并承载启动自动复原（active 行按原 token 经 bind 重绑复活，链接不变）。
// E2E 密钥明文入表（本地库本含全部内容，与密钥只经链接 fragment 分发互不冲突）。
type ShareRecord struct {
	*model.BaseEntity        // 嵌入基础实体
	ShareID           string `gorm:"column:share_id;uniqueIndex" json:"shareId"`         // 业务键（share-{毫秒时间戳}）
	Token             string `gorm:"column:token" json:"token"`                          // 中继会话 token（复原 bind 凭证）
	Title             string `gorm:"column:title" json:"title"`                          // 落地页标题
	WorkIDs           string `gorm:"column:work_ids" json:"workIds"`                     // 分享作品 ID 集（JSON 数组）
	WorkSetIDs        string `gorm:"column:work_set_ids" json:"workSetIds"`              // 分享作品集 ID 集（JSON 数组）
	RelayAddress      string `gorm:"column:relay_address" json:"relayAddress"`           // 中继地址（链接 host 形态）
	KeyB64            string `gorm:"column:key" json:"key"`                              // E2E 密钥（base64url，与链接 fragment 同形态）
	PasswordProtected bool   `gorm:"column:password_protected" json:"passwordProtected"` // 是否有访问密码（密码摘要属运行时载荷，不落表）
	ExpireSeconds     int64  `gorm:"column:expire_seconds" json:"expireSeconds"`         // 发布时有效期配置（-1=中继默认/0=无限期/>0=自定义秒）
	ExpiresAt         int64  `gorm:"column:expires_at" json:"expiresAt"`                 // 到期时刻（unix 毫秒，0=无限期；中继 WELCOME 回填）
	FileCount         int64  `gorm:"column:file_count" json:"fileCount"`                 // 规划白名单文件数（不含缺失）
	TotalBytes        int64  `gorm:"column:total_bytes" json:"totalBytes"`               // 规划白名单文件总字节
	MissingFiles      int64  `gorm:"column:missing_files" json:"missingFiles"`           // 规划时源文件缺失条数
	State             string `gorm:"column:state" json:"state"`                          // active/revoked/expired/failed
	ErrMsg            string `gorm:"column:err_msg" json:"errMsg"`                       // failed 终态的失败原因
	RevokedAt         int64  `gorm:"column:revoked_at" json:"revokedAt"`                 // 撤销时刻（unix 毫秒，0=未撤销）
}

// NewShareRecord 创建分享记录
func NewShareRecord() *ShareRecord {
	return &ShareRecord{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (ShareRecord) TableName() string {
	return "share_record"
}
