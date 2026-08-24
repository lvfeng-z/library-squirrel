package backup

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// idChunkSize ID 集分块的块大小：SQLite 绑定参数上限（旧默认 999）内的保守取值，
// 超 500 个 ID 的 IN/NOT IN 拆多块拼装
const idChunkSize = 500

// BackupRepository 备份仓储实现（纯保管清单 CRUD，按主键为唯一查询面）
type BackupRepository struct {
	*database.BaseRepository[entity.Backup]
}

// NewRepository 创建备份仓储
func NewRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{
		BaseRepository: database.NewBaseRepository[entity.Backup](db),
	}
}

// ListCreatedBefore 查询创建时间早于 beforeMs（毫秒）的清单行（backup 表无软删，普通查询；
// 供备份治理正向无主候选扫描）
func (r *BackupRepository) ListCreatedBefore(ctx context.Context, beforeMs int64) ([]*entity.Backup, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Lt{Column: "create_time", Value: beforeMs},
		},
	}
	return r.List(ctx, opt)
}

// ListAllIDs 全量投影清单行 ID（BaseRepository 无投影面故链式表达；供备份治理反向悬空判定的现存集）
func (r *BackupRepository) ListAllIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).Pluck("id", &ids).Error
	return ids, err
}

// GetByFilePathInWorkDir 按保管路径精确查指定工作目录的清单行，无命中返回 (nil, nil)。
// 供 fsmonitor backup 域：文件 Remove 事件按路径定位清单行。workdir 过滤排除
// 工作目录迁移前的旧行（其文件不在当前监控树内，路径字符串可能撞车）
func (r *BackupRepository) GetByFilePathInWorkDir(ctx context.Context, workDir string, filePath string) (*entity.Backup, error) {
	var row entity.Backup
	err := r.GORM().WithContext(ctx).Where("workdir = ? AND file_path = ?", workDir, filePath).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListByPathPrefixInWorkDir 按路径前缀查指定工作目录的清单行（含多级下级）。
// 供 fsmonitor backup 域：目录 Remove 事件按前缀圈定受影响清单行。
// prefix 为目录路径（正斜杠），匹配 prefix + "/" 下级
func (r *BackupRepository) ListByPathPrefixInWorkDir(ctx context.Context, workDir string, prefix string) ([]*entity.Backup, error) {
	rows := make([]*entity.Backup, 0)
	err := r.GORM().WithContext(ctx).
		Where("workdir = ? AND file_path LIKE ?", workDir, prefix+"/%").
		Find(&rows).Error
	return rows, err
}

// ListAllInWorkDir 全量查指定工作目录中保管路径有效的清单行（file_path 非空）。
// 供 fsmonitor backup 域离线对账：清单行 × 磁盘文件比对的数据源
func (r *BackupRepository) ListAllInWorkDir(ctx context.Context, workDir string) ([]*entity.Backup, error) {
	rows := make([]*entity.Backup, 0)
	err := r.GORM().WithContext(ctx).
		Where("workdir = ? AND file_path IS NOT NULL AND file_path != ''", workDir).
		Find(&rows).Error
	return rows, err
}

// UpdateFilePath 更新清单行保管路径（fsmonitor backup 域移动同步：行路径跟随文件新位置）
func (r *BackupRepository) UpdateFilePath(ctx context.Context, id int64, filePath string) error {
	return r.GORM().WithContext(ctx).Model(&entity.Backup{}).Where("id = ?", id).Update("file_path", filePath).Error
}

// NormalizeFilePaths 规范化 file_path 分隔符为正斜杠（历史行曾以反斜杠入库，与
// fsmonitor backup 域对账的正斜杠磁盘键、事件路径永不匹配即恒判缺失），返回修正行数
func (r *BackupRepository) NormalizeFilePaths(ctx context.Context) (int64, error) {
	result := r.GORM().WithContext(ctx).Model(&entity.Backup{}).
		Where("file_path LIKE ?", `%\%`).
		Update("file_path", gorm.Expr(`replace(file_path, '\', '/')`))
	return result.RowsAffected, result.Error
}

// PageBackups 分页查询保管清单（create_time 倒序固定序）。
// includeIDs/excludeIDs 为 ID 集过滤（引用态过滤由调用方把引用集折算成 ID 集传入，仓储不感知引用语义）：
// nil=无该向过滤；非 nil 空集 include=恒空结果（引用态=有主但当前零引用）、空集 exclude 无作用。
// 大 ID 集分块拼 IN/NOT IN，避免触碰 SQLite 绑定参数上限
func (r *BackupRepository) PageBackups(ctx context.Context, pageNumber, pageSize int, includeIDs []int64, excludeIDs []int64) (*model.Page[entity.Backup], error) {
	if includeIDs != nil && len(includeIDs) == 0 {
		return model.NewPage[entity.Backup]([]*entity.Backup{}, 0, pageNumber, pageSize), nil
	}
	conds := make([]clause.Expression, 0, len(includeIDs)/idChunkSize+len(excludeIDs)/idChunkSize+2)
	if len(includeIDs) > 0 {
		chunks := make([]clause.Expression, 0, (len(includeIDs)+idChunkSize-1)/idChunkSize)
		for start := 0; start < len(includeIDs); start += idChunkSize {
			end := min(start+idChunkSize, len(includeIDs))
			chunks = append(chunks, clause.IN{Column: clause.PrimaryColumn, Values: toAnySlice(includeIDs[start:end])})
		}
		conds = append(conds, clause.Or(chunks...))
	}
	for start := 0; start < len(excludeIDs); start += idChunkSize {
		end := min(start+idChunkSize, len(excludeIDs))
		conds = append(conds, clause.Not(clause.IN{Column: clause.PrimaryColumn, Values: toAnySlice(excludeIDs[start:end])}))
	}
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conds,
			OrderBy: []clause.Expression{
				clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "create_time"}, Desc: true}}},
			},
		},
		Page:     pageNumber,
		PageSize: pageSize,
	}
	return r.Page(ctx, opt)
}

// toAnySlice int64 切片转 any 切片（GORM clause.Values 形参）
func toAnySlice(ids []int64) []any {
	values := make([]any, len(ids))
	for i, id := range ids {
		values[i] = id
	}
	return values
}
