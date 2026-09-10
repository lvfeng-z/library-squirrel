package entity

import (
	"fmt"

	"github.com/library-squirrel/backend/base/model"
)

// ExportTask 导出任务领域行：导出任务的选择参数载体，与所属 task 核心行 1:1——
// 主键 ID 即所属 task.id（共享主键值，无独立任务外键列）
type ExportTask struct {
	*model.BaseEntity     // ID 恒 = 所属 task.id
	WorkIDs       string `gorm:"column:work_ids" json:"workIds"`        // 选择的作品 ID 集(JSON 数组文本;恒序列化,空集存 [])
	WorkSetIDs    string `gorm:"column:work_set_ids" json:"workSetIds"` // 选择的作品集 ID 集(JSON 数组文本;恒序列化,空集存 [])
	OutputDir     string `gorm:"column:output_dir" json:"outputDir"`    // 输出目录原值;空串=执行时取 workDir 根
}

// NewExportTask 创建导出任务领域行，taskID 为所属 task 行 id，构造即赋共享主键。
// 非正 id 直接 panic：零值主键插入会被 SQLite 静默按 rowid 分配新值，
// 破坏与 task 行的 1:1 同值约束且不报错，故在构造口 fail-fast
func NewExportTask(taskID int64) *ExportTask {
	if taskID <= 0 {
		panic(fmt.Sprintf("NewExportTask: 所属任务 ID 须为正数，实际 %d", taskID))
	}
	return &ExportTask{
		BaseEntity: &model.BaseEntity{ID: taskID},
	}
}

// TableName 指定表名
func (ExportTask) TableName() string {
	return "export_task"
}
