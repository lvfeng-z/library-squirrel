package task

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShareTaskRepository 分享接收任务领域行仓储。BaseRepository 以私有字段组合——
// 通用写方法（可直写任意主键的 Create/Save/Updates）不外漏，写路径收口到 CreateForTask 单口
type ShareTaskRepository struct {
	base *database.BaseRepository[entity.ShareTask]
}

// NewShareTaskRepository 创建分享接收任务领域行仓储
func NewShareTaskRepository(db *gorm.DB) *ShareTaskRepository {
	return &ShareTaskRepository{
		base: database.NewBaseRepository[entity.ShareTask](db),
	}
}

// CreateForTask 为任务 taskID 建分享接收任务领域行。领域行主键与核心行共享值：
// 入参对象携带的 id 一律覆写为 taskID（不信任调用方赋值）；非正任务 id 拒绝写入
func (r *ShareTaskRepository) CreateForTask(ctx context.Context, taskID int64, st *entity.ShareTask) error {
	if taskID <= 0 {
		return fmt.Errorf("创建 share_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	st.SetID(taskID)
	fillSharedKeyTimestamps(st.BaseEntity)
	return r.base.Create(ctx, st)
}

// GetById 按共享主键（=所属任务 id）查询领域行
func (r *ShareTaskRepository) GetById(ctx context.Context, id int64) (*entity.ShareTask, error) {
	return r.base.GetById(ctx, id)
}

// ListByIds 按共享主键集合批量查询领域行（树双查的领域行装配步）
func (r *ShareTaskRepository) ListByIds(ctx context.Context, ids []int64) (map[int64]*entity.ShareTask, error) {
	result := make(map[int64]*entity.ShareTask, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	vals := make([]interface{}, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	rows, err := r.base.List(ctx, &database.QueryOption{
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

// fillSharedKeyTimestamps 共享主键非零时 BaseRepository.Create/Save 不补 CreateTime
// （其自动填充以零主键为条件），零值时间戳在此补齐；调用方已带核心行时间戳（同事务同值场景）则不覆盖
func fillSharedKeyTimestamps(base *model.BaseEntity) {
	if base.GetCreateTime() == 0 {
		base.SetCreateTime(util.GetCurrentTimestamp())
	}
}
