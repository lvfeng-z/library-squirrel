package download

// 板块重执行选择写行：重下载入口的领域行写入。父任务请求展开到全部子成员（任务树两级：
// 父→叶子），各子任务持有板块选择供执行派生与单独续传读取。

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/library-squirrel/backend/base/model/entity"
)

// TaskChildReader 任务核心行子成员查询（task 仓储实现，装配注入）：写行范围的任务树展开用
type TaskChildReader interface {
	// ListChildrenTask 查询父任务的直接子任务（叶子级）
	ListChildrenTask(ctx context.Context, pid int64) ([]*entity.Task, error)
}

// SectionRecorder 板块重执行选择写行器（消费方接口定义在 taskManager 的重下载入口，
// 本类型经装配注入满足）
type SectionRecorder struct {
	workTasks *WorkTaskRepository
	children  TaskChildReader
}

// NewSectionRecorder 创建板块重执行选择写行器
func NewSectionRecorder(workTasks *WorkTaskRepository, children TaskChildReader) *SectionRecorder {
	return &SectionRecorder{workTasks: workTasks, children: children}
}

// RecordSections 把板块选择（store_roles 三态 + include_work_info）写入各任务的作品任务领域行。
// 每个请求 id 连同其子成员一并写入：父任务请求展开到全部子成员；叶子/独立任务请求只写自身
func (r *SectionRecorder) RecordSections(ctx context.Context, taskIds []int64, storeRoles sql.NullString, includeWorkInfo bool) error {
	ids := make([]int64, 0, len(taskIds)*2)
	for _, id := range taskIds {
		ids = append(ids, id)
		children, err := r.children.ListChildrenTask(ctx, id)
		if err != nil {
			return fmt.Errorf("查询任务 %d 子成员失败: %w", id, err)
		}
		for _, c := range children {
			ids = append(ids, c.GetID())
		}
	}
	return r.workTasks.UpdateRedownloadSections(ctx, ids, storeRoles, includeWorkInfo)
}
