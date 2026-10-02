package importer

import (
	"context"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

// ImportTaskRepository 导入任务领域行仓储。BaseRepository 以私有字段组合——
// 通用写方法（可直写任意主键的 Create/Save/Updates）不外漏，写路径收口到 CreateForTask 单口
type ImportTaskRepository struct {
	base *database.BaseRepository[entity.ImportTask]
}

// NewImportTaskRepository 创建导入任务领域行仓储
func NewImportTaskRepository(db *gorm.DB) *ImportTaskRepository {
	return &ImportTaskRepository{
		base: database.NewBaseRepository[entity.ImportTask](db),
	}
}

// CreateForTask 为任务 taskID 建导入任务领域行。领域行主键与核心行共享值：
// 入参对象携带的 id 一律覆写为 taskID（不信任调用方赋值）；非正任务 id 拒绝写入
func (r *ImportTaskRepository) CreateForTask(ctx context.Context, taskID int64, it *entity.ImportTask) error {
	if taskID <= 0 {
		return fmt.Errorf("创建 import_task 领域行失败: 所属任务 ID 须为正数，实际 %d", taskID)
	}
	it.SetID(taskID)
	fillImportTaskTimestamps(it.BaseEntity)
	return r.base.Create(ctx, it)
}

// GetById 按共享主键（=所属任务 id）查询领域行
func (r *ImportTaskRepository) GetById(ctx context.Context, id int64) (*entity.ImportTask, error) {
	return r.base.GetById(ctx, id)
}

// fillImportTaskTimestamps 共享主键非零时 BaseRepository.Create/Save 不补 CreateTime
// （其自动填充以零主键为条件），零值时间戳在此补齐；调用方已带核心行时间戳（同事务同值场景）则不覆盖
func fillImportTaskTimestamps(base *model.BaseEntity) {
	if base.GetCreateTime() == 0 {
		base.SetCreateTime(util.GetCurrentTimestamp())
	}
}
