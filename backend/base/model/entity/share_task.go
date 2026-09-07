package entity

import (
	"fmt"

	"github.com/library-squirrel/backend/base/model"
)

// ShareTask 分享接收任务领域行：收件任务的连接与清单参数载体，与所属 task 核心行 1:1——
// 主键 ID 即所属 task.id（共享主键值，无独立任务外键列），行集仅收件子任务行（父容器行是纯控制行，不建领域行）
type ShareTask struct {
	*model.BaseEntity        // ID 恒 = 所属 task.id
	RelayDial         string `gorm:"column:relay_dial" json:"relayDial"`       // 中继 TCP 拨号地址(host:port)
	RelayHost         string `gorm:"column:relay_host" json:"relayHost"`       // 中继展示地址
	Token             string `gorm:"column:token" json:"token"`                // 会话 token
	KeyB64            string `gorm:"column:key_b64" json:"keyB64"`             // E2E 密钥(base64url)
	PasswordHash      string `gorm:"column:password_hash" json:"passwordHash"` // 访问密码摘要(sha256 hex);空=无密码
	ManifestPath      string `gorm:"column:manifest_path" json:"manifestPath"` // 共享清单 workDir 相对路径(正斜杠 relPath 域)
	ManifestID        int64  `gorm:"column:manifest_id" json:"manifestId"`     // 本任务负责的作品 ID;0=过时载荷,执行面按显式 Fail 处置
}

// NewShareTask 创建分享接收任务领域行，taskID 为所属 task 行 id，构造即赋共享主键。
// 非正 id 直接 panic：零值主键插入会被 SQLite 静默按 rowid 分配新值，
// 破坏与 task 行的 1:1 同值约束且不报错，故在构造口 fail-fast
func NewShareTask(taskID int64) *ShareTask {
	if taskID <= 0 {
		panic(fmt.Sprintf("NewShareTask: 所属任务 ID 须为正数，实际 %d", taskID))
	}
	return &ShareTask{
		BaseEntity: &model.BaseEntity{ID: taskID},
	}
}

// TableName 指定表名
func (ShareTask) TableName() string {
	return "share_task"
}
