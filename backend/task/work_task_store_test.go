package task

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 本文件为作品任务领域行存取的测试替身：领域行仓储外置后，task 模块测试经窄接口
// （WorkTaskWriter/WorkTaskReader）注入本替身，真实落库/读库 work_task 行。
// 主键覆写与时间戳补齐语义与生产仓储一致（领域行仓储自身的守卫测试在其所在模块锚定）。

// testWorkTaskStore 作品任务领域行存取替身（与生产仓储同底座：BaseRepository 直写 + 共享主键覆写）
type testWorkTaskStore struct {
	base *database.BaseRepository[entity.WorkTask]
}

// newTestWorkTaskStore 构建作品任务领域行存取替身（同一测试库）
func newTestWorkTaskStore(db *gorm.DB) *testWorkTaskStore {
	return &testWorkTaskStore{base: database.NewBaseRepository[entity.WorkTask](db)}
}

// CreateForTask 为任务 taskID 建作品任务领域行（入参 id 覆写为 taskID；非正任务 id 拒绝）
func (s *testWorkTaskStore) CreateForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("创建 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	fillSharedKeyTimestamps(wt.BaseEntity)
	return s.base.Create(ctx, wt)
}

// CreateBatchForTask 批量建作品任务领域行（各领域行须已持核心行共享主键）
func (s *testWorkTaskStore) CreateBatchForTask(ctx context.Context, wts []*entity.WorkTask) error {
	if len(wts) == 0 {
		return nil
	}
	for _, wt := range wts {
		if wt.GetID() <= 0 {
			return fmt.Errorf("批量创建 work_task 领域行失败: 领域行须已持核心行 id，实际 %d", wt.GetID())
		}
		fillSharedKeyTimestamps(wt.BaseEntity)
	}
	return s.base.CreateBatch(ctx, wts)
}

// SaveForTask 为任务 taskID 全字段 UPSERT 作品任务领域行
func (s *testWorkTaskStore) SaveForTask(ctx context.Context, taskID int64, wt *entity.WorkTask) error {
	if taskID <= 0 {
		return fmt.Errorf("保存 work_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	wt.SetID(taskID)
	fillSharedKeyTimestamps(wt.BaseEntity)
	return s.base.Save(ctx, wt)
}

// GetById 按共享主键（=所属任务 id）查询领域行
func (s *testWorkTaskStore) GetById(ctx context.Context, id int64) (*entity.WorkTask, error) {
	return s.base.GetById(ctx, id)
}

// ListByIds 按共享主键集合批量查询领域行
func (s *testWorkTaskStore) ListByIds(ctx context.Context, ids []int64) (map[int64]*entity.WorkTask, error) {
	result := make(map[int64]*entity.WorkTask, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	vals := make([]interface{}, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	rows, err := s.base.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: clause.Column{Name: "id"}, Values: vals}},
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.GetID()] = row
	}
	return result, nil
}
