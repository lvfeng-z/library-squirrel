package entity

import (
	"fmt"

	"github.com/library-squirrel/backend/base/model"
)

// ImportTask 导入任务领域行：zip 导入子任务的载荷载体，与所属 task 核心行 1:1——
// 主键 ID 即所属 task.id（共享主键值，无独立任务外键列）。
// 仅子任务建行：父容器「导入（N 项）」为纯控制行不建领域行（对齐 share_task 先例）
type ImportTask struct {
	*model.BaseEntity // ID 恒 = 所属 task.id
	ZipPath     string `gorm:"column:zip_path" json:"zipPath"`     // 导入包路径原值（子任务执行时打开取文件字节）
	ManifestRel string `gorm:"column:manifest_rel" json:"manifestRel"` // 共享 manifest 的 workDir 相对路径（正斜杠；建树时从包内解出落盘的位置）
	ManifestID  int64  `gorm:"column:manifest_id" json:"manifestId"`   // 本任务负责的 manifest 作品 ID（0=过时载荷判据，对齐 share_task 的 ManifestID==0 判过时）
}

// NewImportTask 创建导入任务领域行，taskID 为所属 task 行 id，构造即赋共享主键。
// 非正 id 直接 panic：零值主键插入会被 SQLite 静默按 rowid 分配新值，
// 破坏与 task 行的 1:1 同值约束且不报错，故在构造口 fail-fast
func NewImportTask(taskID int64) *ImportTask {
	if taskID <= 0 {
		panic(fmt.Sprintf("NewImportTask: 所属任务 ID 须为正数，实际 %d", taskID))
	}
	return &ImportTask{
		BaseEntity: &model.BaseEntity{ID: taskID},
	}
}

// TableName 指定表名
func (ImportTask) TableName() string {
	return "import_task"
}
