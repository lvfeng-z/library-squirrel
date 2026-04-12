package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BaseRepository 泛型基础仓储 GORM 实现
// 具体模块的数据库操作模块通过嵌入它来获得基础 CRUD 实现
// T 应嵌入 *model.BaseEntity 以获得 ID、CreateTime、UpdateTime 字段
type BaseRepository[T model.Entity] struct {
	db *gorm.DB
}

// NewBaseRepository 创建基础仓储实例
func NewBaseRepository[T model.Entity](db *gorm.DB) *BaseRepository[T] {
	return &BaseRepository[T]{db: db}
}

// ========== 字段名映射 ==========

// ErrFieldNotFound 字段不存在错误
type ErrFieldNotFound struct {
	TypeName  string
	FieldName string
}

func (e *ErrFieldNotFound) Error() string {
	return fmt.Sprintf("field %q not found in type %q", e.FieldName, e.TypeName)
}

// ErrNoColumnTag 字段没有 gorm column 标签错误
type ErrNoColumnTag struct {
	TypeName  string
	FieldName string
}

func (e *ErrNoColumnTag) Error() string {
	return fmt.Sprintf("field %q in type %q has no gorm column tag", e.FieldName, e.TypeName)
}

// GetColumnName 获取结构体字段的 gorm column 标签值
// 如果字段不存在或没有 column 标签，返回错误
func GetColumnName[T any](fieldName string) (string, error) {
	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	field, found := typ.FieldByName(fieldName)
	if !found {
		typeName := typ.Name()
		if typeName == "" {
			typeName = fmt.Sprintf("%T", t)
		}
		return "", &ErrFieldNotFound{TypeName: typeName, FieldName: fieldName}
	}

	tag := field.Tag.Get("gorm")
	if tag == "" {
		typeName := typ.Name()
		if typeName == "" {
			typeName = fmt.Sprintf("%T", t)
		}
		return "", &ErrNoColumnTag{TypeName: typeName, FieldName: fieldName}
	}

	// 解析 gorm 标签，格式如 "column:site_tag_name;uniqueIndex:..."
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "column:") {
			return strings.TrimPrefix(part, "column:"), nil
		}
	}

	typeName := typ.Name()
	if typeName == "" {
		typeName = fmt.Sprintf("%T", t)
	}
	return "", &ErrNoColumnTag{TypeName: typeName, FieldName: fieldName}
}

// ========== CRUD 方法 ==========

// Save 保存单个实体
func (r *BaseRepository[T]) Save(ctx context.Context, entity *T) error {
	now := util.GetCurrentTimestamp()
	e := *entity
	if e.GetID() == 0 {
		e.SetCreateTime(now)
	}
	e.SetUpdateTime(now)
	return r.db.WithContext(ctx).Create(entity).Error
}

// SaveBatch 批量保存
func (r *BaseRepository[T]) SaveBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, entity := range entities {
		e := *entity
		if e.GetID() == 0 {
			e.SetCreateTime(now)
		}
		e.SetUpdateTime(now)
	}
	return r.db.WithContext(ctx).Create(entities).Error
}

// Delete 根据ID删除
func (r *BaseRepository[T]) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(T), id).Error
}

// DeleteBatch 批量删除
func (r *BaseRepository[T]) DeleteBatch(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Delete(new(T), ids).Error
}

// Update 更新实体
func (r *BaseRepository[T]) Update(ctx context.Context, entity *T) error {
	e := *entity
	e.SetUpdateTime(util.GetCurrentTimestamp())
	return r.db.WithContext(ctx).Save(entity).Error
}

// UpdateBatch 批量更新
func (r *BaseRepository[T]) UpdateBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, entity := range entities {
		e := *entity
		e.SetUpdateTime(now)
	}
	return r.db.WithContext(ctx).Save(entities).Error
}

// GetById 根据ID获取
func (r *BaseRepository[T]) GetById(ctx context.Context, id int64) (*T, error) {
	var entity T
	err := r.db.WithContext(ctx).First(&entity, id).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// Get 根据查询条件获取单个
// conditions: WHERE 条件切片，支持多个条件 AND 连接
// orderBy: ORDER BY 排序表达式
func (r *BaseRepository[T]) Get(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression) (*T, error) {
	var entity T
	db := r.db.WithContext(ctx).Model(new(T))
	if len(conditions) > 0 {
		for _, cond := range conditions {
			db = db.Clauses(cond)
		}
	}
	if orderBy != nil {
		db = db.Clauses(orderBy)
	}
	err := db.First(&entity).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// List 根据查询条件获取列表
// conditions: WHERE 条件切片，支持多个条件 AND 连接
// orderBy: ORDER BY 排序表达式
func (r *BaseRepository[T]) List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*T, error) {
	var entities []*T
	db := r.db.WithContext(ctx).Model(new(T))
	if len(conditions) > 0 {
		for _, cond := range conditions {
			db = db.Clauses(cond)
		}
	}
	if orderBy != nil {
		db = db.Clauses(orderBy)
	}
	if limit > 0 {
		db = db.Clauses(clause.Limit{Limit: &limit, Offset: offset})
	}
	err := db.Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// Count 统计数量
// conditions: WHERE 条件切片，支持多个条件 AND 连接
func (r *BaseRepository[T]) Count(ctx context.Context, conditions []clause.Expression) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(T))
	if len(conditions) > 0 {
		for _, cond := range conditions {
			db = db.Clauses(cond)
		}
	}
	err := db.Count(&count).Error
	return count, err
}

// Page 分页查询
// conditions: WHERE 条件切片，支持多个条件 AND 连接
// orderBy: ORDER BY 排序表达式
func (r *BaseRepository[T]) Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) ([]*T, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 查询列表
	list, err := r.List(ctx, conditions, orderBy, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	// 统计总数
	total, err := r.Count(ctx, conditions)
	if err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// GetDB 获取底层 GORM DB 实例（供特殊查询使用）
func (r *BaseRepository[T]) GetDB() *gorm.DB {
	return r.db
}

// GORM 获取底层 GORM DB 实例（别名）
func (r *BaseRepository[T]) GORM() *gorm.DB {
	return r.db
}

// Transaction 执行事务
func (r *BaseRepository[T]) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// ExecRawSQL 执行原生 SQL（仅用于复杂查询）
func (r *BaseRepository[T]) ExecRawSQL(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return r.db.WithContext(ctx).Raw(query, args...)
}

// ========== 辅助方法 ==========

// Create 创建（别名，用于特定场景）
func (r *BaseRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.Save(ctx, entity)
}

// Updates 更新（仅更新非零字段）
func (r *BaseRepository[T]) Updates(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Model(new(T)).Updates(entity).Error
}

// DeleteByIds 根据IDs删除（别名）
func (r *BaseRepository[T]) DeleteByIds(ctx context.Context, ids []int64) error {
	return r.DeleteBatch(ctx, ids)
}

// FindAll 查询所有
func (r *BaseRepository[T]) FindAll(ctx context.Context) ([]*T, error) {
	var entities []*T
	err := r.db.WithContext(ctx).Find(&entities).Error
	return entities, err
}

// FindOne 查询单个（带排序）
func (r *BaseRepository[T]) FindOne(ctx context.Context, orderBy string, ascending bool) (*T, error) {
	var entity T
	query := r.db.WithContext(ctx).Model(new(T))
	if orderBy != "" {
		dir := "ASC"
		if !ascending {
			dir = "DESC"
		}
		query = query.Order(orderBy + " " + dir)
	}
	err := query.First(&entity).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// String 格式化输出（调试用）
func (r *BaseRepository[T]) String() string {
	return fmt.Sprintf("BaseRepository<%T>", *new(T))
}
