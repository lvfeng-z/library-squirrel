package fsmonitor

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// cursorRepository fsmonitor_cursor 表仓储，实现 CursorStore。
// 嵌入 BaseRepository 获得事务感知（getDb→DBFromContext），支持调用方事务内「续读 dispatch + 落游标」原子化（D6）。
// fsmonitor 自有数据（非 persistentStore），故 repository 内聚于本包；service 层仍经 CursorStore 接口消费、不直接 import database。
type cursorRepository struct {
	*database.BaseRepository[domain.FsmonitorCursor]
}

// NewCursorRepository 创建 USN 游标仓储（由 app.go 注入 Deps.CursorStore）。
func NewCursorRepository(db *gorm.DB) CursorStore {
	return &cursorRepository{BaseRepository: database.NewBaseRepository[domain.FsmonitorCursor](db)}
}

// Get 读取 (journalID, workDir) 绑定的游标；无记录返回 nil。
func (r *cursorRepository) Get(ctx context.Context, journalID uint64, workDir string) (*Cursor, error) {
	e, err := r.find(ctx, journalID, workDir)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, nil
	}
	return &Cursor{JournalID: e.JournalID, StartUsn: e.StartUsn, WorkDir: e.WorkDir}, nil
}

// Save upsert 游标：(journalID, workDir) 已存在则更新 start_usn（BaseRepository.Updates 设 UpdateTime），
// 否则新建（BaseRepository.Create 设 CreateTime/UpdateTime）。start_usn 恒为有效 USN（非零），Updates 非零字段语义适用。
func (r *cursorRepository) Save(ctx context.Context, c Cursor) error {
	existing, err := r.find(ctx, c.JournalID, c.WorkDir)
	if err != nil {
		return err
	}
	if existing != nil {
		existing.StartUsn = c.StartUsn
		return r.Updates(ctx, existing)
	}
	e := domain.NewFsmonitorCursor()
	e.JournalID = c.JournalID
	e.StartUsn = c.StartUsn
	e.WorkDir = c.WorkDir
	return r.Create(ctx, e)
}

// find 按 (journalID, workDir) 唯一键查记录，无命中返回 nil（用 List 而非 First，避免 ErrRecordNotFound 误报错）。
func (r *cursorRepository) find(ctx context.Context, journalID uint64, workDir string) (*domain.FsmonitorCursor, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "journal_id", Value: journalID},
			clause.Eq{Column: "work_dir", Value: workDir},
		},
		Limit: 1,
	}
	list, err := r.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}
