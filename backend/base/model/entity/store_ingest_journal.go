package entity

import (
	"errors"

	"github.com/library-squirrel/backend/base/model"
)

// IngestAbortAction 入库撤回处置声明：文件已落位而入库未提交时对文件的处置方式。
// 该判据只有调用方持有（该轨能否续传、是否可重产），且收口可能发生在进程重启后（调用方
// 内存上下文已不存在），故随登记行持久化，撤回与启动恢复只执行声明、不推断
type IngestAbortAction string

const (
	// AbortActionReturnToStaging 退回暂存：文件移回登记的暂存位置，保住续传/重试语义
	AbortActionReturnToStaging IngestAbortAction = "returnToStaging"
	// AbortActionDiscard 丢弃：删除文件（一次性产物，重产成本低于保留）
	AbortActionDiscard IngestAbortAction = "discard"
)

// 处置声明校验错误
var (
	// ErrIngestAbortActionUndeclared 处置声明缺失（登记时必填，不给默认值）
	ErrIngestAbortActionUndeclared = errors.New("入库撤回处置未声明")
	// ErrIngestAbortActionInvalid 处置声明为未定义取值
	ErrIngestAbortActionInvalid = errors.New("入库撤回处置非法（取值 returnToStaging 或 discard）")
)

// String 返回处置声明的存储字符串
func (a IngestAbortAction) String() string {
	return string(a)
}

// Validate 处置声明合法性判定：空串或未定义取值即拒绝（必填无默认——默认值等于替调用方猜处置）
func (a IngestAbortAction) Validate() error {
	switch a {
	case AbortActionReturnToStaging, AbortActionDiscard:
		return nil
	case "":
		return ErrIngestAbortActionUndeclared
	default:
		return ErrIngestAbortActionInvalid
	}
}

// ParseIngestAbortAction 把存储字符串解析为处置声明（数据库行读取/调用方入参 → 枚举）
func ParseIngestAbortAction(s string) (IngestAbortAction, error) {
	action := IngestAbortAction(s)
	return action, action.Validate()
}

// StoreIngestJournal 入库登记行：persistent_store 入库动作的自有账本，为跨崩溃的入库
// 原子性留下痕迹。登记行存在 ⟺ 该次入库未提交——建 persistent_store 行与删登记行在
// 同一事务内完成，状态由此构造性地表达，无状态列；行只引用文件路径不引用数据库行，无外键；
// 登记行消亡即物理 DELETE，无软删语义
type StoreIngestJournal struct {
	*model.BaseEntity
	// FilePath 本次入库的最终落位路径（relPath 域：workDir 相对、正斜杠，须命中 store/ 白名单）
	FilePath string `gorm:"column:file_path;not null" json:"filePath"`
	// StagingPath 内容当前所在的暂存位置（relPath 域；处置为退回暂存时的退回目标）
	StagingPath string `gorm:"column:staging_path;not null" json:"stagingPath"`
	// Workdir 登记时的库根（恢复时判定登记行是否属于当前库：不匹配行保留不动）
	Workdir string `gorm:"column:workdir;not null" json:"workdir"`
	// AbortAction 调用方在登记时声明的撤回处置（两态枚举，必填）
	AbortAction IngestAbortAction `gorm:"column:abort_action;not null" json:"abortAction"`
}

func (StoreIngestJournal) TableName() string {
	return "store_ingest_journal"
}

func NewStoreIngestJournal() *StoreIngestJournal {
	return &StoreIngestJournal{
		BaseEntity: &model.BaseEntity{},
	}
}
