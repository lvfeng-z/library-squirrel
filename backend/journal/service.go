package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// 轮转参数（常量化；可配置需求出现时再议）。取值口径：TTL 365 天 = 消费方可触及约一年
// 历史（游标休眠近一年不过期）；容量 20 万行 ≈ 548 事件/天持续一年的兜底上限，正常
// 事件速率下 TTL 先于容量生效，容量轨仅作异常速率的保活封顶
const (
	// RotationTTL 轮转 TTL：账行创建时刻早于该年龄即被裁剪
	RotationTTL = 365 * 24 * time.Hour
	// RotationMaxRows 轮转容量上限：存活账行数超过该值时裁最旧
	RotationMaxRows = 200000
)

// defaultListLimit ListSince 未给有效限量时的缺省每批行数
const defaultListLimit = 100

// 写入校验错误
var (
	// ErrDomainEmpty 记账域为空（域是账行的目录归属，缺失即无法归类）
	ErrDomainEmpty = errors.New("记账域不能为空")
	// ErrEventTypeEmpty 事件类型为空（类型是域内的事件分类，缺失即无法辨识记了什么）
	ErrEventTypeEmpty = errors.New("记账事件类型不能为空")
	// ErrSubjectIDInvalid 主体引用非法（任务/作品等域内主键从 1 起，非正数即写入方未携带主体）
	ErrSubjectIDInvalid = errors.New("记账主体引用必须为正数")
)

// ListSnapshot ListSince 的库层快照：同一读事务内的最旧存活游标与 cursor 后行集。
// 轮转裁剪与游标读取并发进行时两读共享同一视图，过期判定与行集不会分别落在裁剪前、裁剪后两个状态
type ListSnapshot struct {
	// Entries cursor 之后按 id 升序的账行（行数可超出调用方限量 1 行，供 HasMore 探测）
	Entries []*entity.JournalEntry
	// OldestAliveID 最旧存活账行 id（AliveExists 为 false 时无意义）
	OldestAliveID int64
	// AliveExists 表中是否存在存活账行（false = 空表）
	AliveExists bool
}

// Repository 记账账表仓储接口（由 service 定义所需的数据库操作，提供方实现）
type Repository interface {
	// Create 落一条账行（自增 id 与 create_time 由仓储层回填）
	Create(ctx context.Context, entry *entity.JournalEntry) error
	// ListSinceSnapshot 快照读取最旧存活游标 + cursor 之后 id 升序的前 limit 行
	ListSinceSnapshot(ctx context.Context, cursor int64, limit int) (*ListSnapshot, error)
	// RotateOldest 轮转裁剪：删除创建时刻早于 expiredBefore（毫秒时间戳）的行，
	// 以及 id 落在最新 keepRows 行之外的行（裁最旧）；返回裁剪行数
	RotateOldest(ctx context.Context, expiredBefore int64, keepRows int) (int64, error)
}

// ListResult ListSince 的返回形态。该契约是外部消费方（外部 AI 操作接口 /api/journal
// 端点）的依赖面，签名稳定优先
type ListResult struct {
	// Entries 本次返回的账行（id 升序；追平后为空切片）
	Entries []*entity.JournalEntry
	// NextCursor 下次读取起点：有返回行时 = 末行 id；无返回行时回传传入游标（消费位置不动）
	NextCursor int64
	// HasMore 游标之后仍有未返回行（true 可立即续读；false = 已读到当前末尾）
	HasMore bool
	// CursorExpired 游标过期标记：游标非 0 且其指向的账行已不可见——表被轮转裁空，或游标
	// 早于最旧存活账行（消费位置及其之前的历史已被裁掉，连续性无法核对）。消费方收到本标记
	// 应回退可推导查询（按 create_time/状态过滤）补缺口，而非把本次行集当完整历史；
	// cursor=0（从头消费）不判过期：新消费从最旧存活行起读，属正常引导
	CursorExpired bool
}

// Service 记账服务：append-only 账表的写入（写前顺带轮转）、游标读取与轮转。
// 写账尽力而为：Append 把落库错误返回调用方记日志（不 panic、不重试风暴）；
// 轮转属保活性维护，失败仅日志告警、不阻断写账
type Service struct {
	repo Repository
}

// NewService 创建记账服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Append 追加一条账行，入口签名（domain 记账域 / eventType 事件类型 / subjectID 主体
// 引用 / payload JSON 摘要载荷）。提交后写、尽力而为：调用方在业务事务成功后调用，
// 写账失败经返回错误交调用方记日志，业务不回滚。记账写入口低频（仅状态类事件落账），
// 逐次顺带承担轮转触发
func (s *Service) Append(ctx context.Context, domain, eventType string, subjectID int64, payload string) error {
	if domain == "" {
		return ErrDomainEmpty
	}
	if eventType == "" {
		return ErrEventTypeEmpty
	}
	if subjectID <= 0 {
		return ErrSubjectIDInvalid
	}
	if _, err := s.Rotate(ctx); err != nil {
		logger.Log.Warnw("记账账表轮转失败（不阻断写账）", "error", err)
	}
	entry := entity.NewJournalEntry()
	entry.Domain = domain
	entry.EventType = eventType
	entry.SubjectID = subjectID
	entry.Payload = payload
	if err := s.repo.Create(ctx, entry); err != nil {
		return fmt.Errorf("写入记账账行失败: %w", err)
	}
	return nil
}

// ListSince 游标读取：返回 cursor 之后（id 严格大于 cursor）的账行集（id 升序）、
// 下一游标、是否还有后续行、游标过期标记。cursor=0 表示从头消费；limit<=0 取缺省批量
func (s *Service) ListSince(ctx context.Context, cursor int64, limit int) (*ListResult, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	// 多拉 1 行探测 HasMore，省一次 COUNT
	snap, err := s.repo.ListSinceSnapshot(ctx, cursor, limit+1)
	if err != nil {
		return nil, fmt.Errorf("读取记账账行失败: %w", err)
	}
	result := &ListResult{
		Entries:       snap.Entries,
		CursorExpired: cursor != 0 && (!snap.AliveExists || cursor < snap.OldestAliveID),
	}
	if len(snap.Entries) > limit {
		result.HasMore = true
		result.Entries = snap.Entries[:limit]
	}
	if n := len(result.Entries); n > 0 {
		result.NextCursor = result.Entries[n-1].GetID()
	} else {
		result.NextCursor = cursor
	}
	return result, nil
}

// Rotate 轮转：按 TTL 与容量裁最旧（Append 顺带触发，也可由启动/定期调度独立调用），
// 返回裁剪行数
func (s *Service) Rotate(ctx context.Context) (int64, error) {
	cutoff := util.GetCurrentTimestamp() - RotationTTL.Milliseconds()
	deleted, err := s.repo.RotateOldest(ctx, cutoff, RotationMaxRows)
	if err != nil {
		return 0, fmt.Errorf("轮转裁剪记账账表失败: %w", err)
	}
	if deleted > 0 {
		logger.Log.Infow("记账账表轮转完成", "deleted", deleted)
	}
	return deleted, nil
}
