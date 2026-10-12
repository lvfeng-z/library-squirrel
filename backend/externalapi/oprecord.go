package externalapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 外部操作记录：接口模块私有的问责账——只记经操作分发层的外部 API 调用（操作标识、脱敏
// 参数摘要、成败、耗时、调用方自报标签），被动记录、无任何事前把关职能。查询面 = 游标
// 增量端点（机器用）与设置页只读分页列表（人用），两者读同一账表

const (
	// OplogPath 外部操作记录增量查询端点路径（GET，查询参数 cursor/limit）
	OplogPath = "/api/oplog"
	// ClientLabelHeader 客户端标签请求头：调用方自报归属（如智能体名称），记录行原样
	// 落库。自报非鉴别——token 才是鉴权凭据，标签只服务问责时的归属提示
	ClientLabelHeader = "X-Client-Label"
	// opRecordRotationTTL 记录轮转 TTL：行创建时刻早于该年龄即被裁剪（与记账账表同一
	// 缺省口径；可配置需求出现时再议）
	opRecordRotationTTL = 180 * 24 * time.Hour
	// opRecordRotationMaxRows 记录轮转容量上限：存活行数超过该值时裁最旧
	opRecordRotationMaxRows = 50000
	// opRecordDefaultListLimit ListSince 未给有效限量时的缺省每批行数
	opRecordDefaultListLimit = 100
	// opRecordMaxParamsLength 参数摘要的最大保留长度（按字符计，超出截断并附截断标记）
	opRecordMaxParamsLength = 512
)

// oplogQueryParamSpec 记录增量查询端点的参数摘要（GET /api/oplog 查询参数形态）
var oplogQueryParamSpec = []OperationParam{
	{Name: "cursor", Type: "int64", Desc: "消费游标：返回 id 严格大于该值的记录行，0 = 从头消费"},
	{Name: "limit", Type: "int", Desc: "本批行数上限（非正数取缺省 100 行）"},
}

// OpRecordSnapshot ListSince 的库层快照：同一读事务内的最旧存活游标与 cursor 后行集。
// 轮转裁剪与游标读取并发进行时两读共享同一视图，过期判定与行集不会分别落在裁剪前、
// 裁剪后两个状态
type OpRecordSnapshot struct {
	// Entries cursor 之后按 id 升序的记录行（行数可超出调用方限量 1 行，供 HasMore 探测）
	Entries []*entity.ExternalApiOpRecord
	// OldestAliveID 最旧存活记录行 id（AliveExists 为 false 时无意义）
	OldestAliveID int64
	// AliveExists 表中是否存在存活记录行（false = 空表）
	AliveExists bool
}

// OpRecordListResult ListSince 的返回形态（与记账账表游标契约同构，读面双游标端点
// 形状一致，消费者按同一套字段消费）
type OpRecordListResult struct {
	// Entries 本次返回的记录行（id 升序；追平后为空切片）
	Entries []*entity.ExternalApiOpRecord
	// NextCursor 下次读取起点：有返回行时 = 末行 id；无返回行时回传传入游标
	NextCursor int64
	// HasMore 游标之后仍有未返回行（true 可立即续读；false = 已读到当前末尾）
	HasMore bool
	// CursorExpired 游标过期标记：游标非 0 且其指向的记录行已不可见——表被轮转裁空，
	// 或游标早于最旧存活行（消费位置及其之前的历史已被裁掉）。cursor=0（从头消费）
	// 不判过期：新消费从最旧存活行起读，属正常引导
	CursorExpired bool
}

// OpRecordRepository 外部操作记录仓储接口（由本模块的记录服务定义所需数据库操作，
// 仓储实现满足）
type OpRecordRepository interface {
	// Create 落一条记录行（自增 id 与 create_time 由仓储层回填）
	Create(ctx context.Context, record *entity.ExternalApiOpRecord) error
	// ListSinceSnapshot 快照读取最旧存活游标 + cursor 之后 id 升序的前 limit 行
	ListSinceSnapshot(ctx context.Context, cursor int64, limit int) (*OpRecordSnapshot, error)
	// RotateOldest 轮转裁剪：删除创建时刻早于 expiredBefore（毫秒时间戳）的行，以及
	// id 落在最新 keepRows 行之外的行（裁最旧）；返回裁剪行数
	RotateOldest(ctx context.Context, expiredBefore int64, keepRows int) (int64, error)
	// PageRecent 分页读取记录行（id 降序 = 最新调用在前，设置页只读列表用）
	PageRecent(ctx context.Context, pageNumber, pageSize int) (*model.Page[entity.ExternalApiOpRecord], error)
}

// opRecordRepository 外部操作记录仓储实现
type opRecordRepository struct {
	*database.BaseRepository[entity.ExternalApiOpRecord]
}

// NewOpRecordRepository 创建外部操作记录仓储
func NewOpRecordRepository(db *gorm.DB) OpRecordRepository {
	return &opRecordRepository{
		BaseRepository: database.NewBaseRepository[entity.ExternalApiOpRecord](db),
	}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *opRecordRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListSinceSnapshot 同一读事务内双读（最旧存活游标边界 + cursor 后行集）：轮转裁剪与
// 游标读取并发时两读共享同一快照，过期判定不会落在裁剪前、行集落在裁剪后的不一致状态
func (r *opRecordRepository) ListSinceSnapshot(ctx context.Context, cursor int64, limit int) (*OpRecordSnapshot, error) {
	snap := &OpRecordSnapshot{}
	err := r.dbFromCtx(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var boundary struct {
			Total int64
			MinID int64
		}
		if err := tx.Raw("SELECT COUNT(*) AS total, COALESCE(MIN(id), 0) AS min_id FROM external_api_op_record").Scan(&boundary).Error; err != nil {
			return err
		}
		snap.AliveExists = boundary.Total > 0
		snap.OldestAliveID = boundary.MinID
		return tx.Where(clause.Gt{Column: "id", Value: cursor}).
			Order("id ASC").
			Limit(limit).
			Find(&snap.Entries).Error
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// RotateOldest 轮转裁剪（单语句，两条件 OR 连接）：删除创建时刻早于 expiredBefore（毫秒
// 时间戳）的超龄行，以及 id 落在最新 keepRows 行之外的行（容量轨裁最旧）。行数不超容量
// 且无超龄行时零命中（幂等）；容量子查询在表行数不足 keepRows 时返回空集，条件不命中
func (r *opRecordRepository) RotateOldest(ctx context.Context, expiredBefore int64, keepRows int) (int64, error) {
	res := r.dbFromCtx(ctx).WithContext(ctx).Exec(`
		DELETE FROM external_api_op_record
		WHERE create_time < ?
		   OR id < (SELECT MIN(id) FROM (SELECT id FROM external_api_op_record ORDER BY id DESC LIMIT 1 OFFSET ?))
	`, expiredBefore, keepRows-1)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// PageRecent 分页读取记录行（id 降序 = 最新调用在前）
func (r *opRecordRepository) PageRecent(ctx context.Context, pageNumber, pageSize int) (*model.Page[entity.ExternalApiOpRecord], error) {
	return r.Page(ctx, &database.PageOption{
		QueryOption: database.QueryOption{
			OrderBy: []clause.Expression{clause.OrderBy{
				Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "id"}, Desc: true}},
			}},
		},
		Page:     pageNumber,
		PageSize: pageSize,
	})
}

// OpRecordStore 外部操作记录的读写方法面（监听器分发层的落账、记录查询端点与设置页
// 只读列表共用；*OpRecordService 满足）
type OpRecordStore interface {
	// Record 落一条记录行（写前顺带轮转；错误交调用方记日志，不影响操作响应语义）
	Record(ctx context.Context, operation, paramsSummary string, success bool, clientLabel string, durationMs int64) error
	// ListSince 游标读取：cursor 之后（id 严格大于 cursor）的记录行集与游标契约字段
	ListSince(ctx context.Context, cursor int64, limit int) (*OpRecordListResult, error)
	// PageRecent 分页读取记录行（最新调用在前）
	PageRecent(ctx context.Context, pageNumber, pageSize int) (*model.Page[entity.ExternalApiOpRecord], error)
}

// OpRecordService 外部操作记录服务：写入（写前顺带轮转）、游标读取与设置页分页读取。
// 落账尽力而为——写入口由分发层在操作完成后调用，失败经返回错误交调用方记日志，
// 操作响应不受影响
type OpRecordService struct {
	repo OpRecordRepository
}

// NewOpRecordService 创建外部操作记录服务
func NewOpRecordService(repo OpRecordRepository) *OpRecordService {
	return &OpRecordService{repo: repo}
}

// Record 落一条外部操作记录并顺带触发轮转（保活性维护，轮转失败仅日志不阻断写账）。
// 写入口低频受分发层并发上限约束，逐次顺带承担轮转触发
func (s *OpRecordService) Record(ctx context.Context, operation, paramsSummary string, success bool, clientLabel string, durationMs int64) error {
	if _, err := s.Rotate(ctx); err != nil {
		logger.Log.Warnw("外部操作记录轮转失败（不阻断写账）", "error", err)
	}
	record := entity.NewExternalApiOpRecord()
	record.Operation = operation
	record.Params = paramsSummary
	record.Success = success
	record.ClientLabel = clientLabel
	record.DurationMs = durationMs
	if err := s.repo.Create(ctx, record); err != nil {
		return fmt.Errorf("写入外部操作记录失败: %w", err)
	}
	return nil
}

// ListSince 游标读取：返回 cursor 之后的记录行集（id 升序）、下一游标、是否还有后续行、
// 游标过期标记。cursor=0 表示从头消费；limit<=0 取缺省批量
func (s *OpRecordService) ListSince(ctx context.Context, cursor int64, limit int) (*OpRecordListResult, error) {
	if limit <= 0 {
		limit = opRecordDefaultListLimit
	}
	// 多拉 1 行探测 HasMore，省一次 COUNT
	snap, err := s.repo.ListSinceSnapshot(ctx, cursor, limit+1)
	if err != nil {
		return nil, fmt.Errorf("读取外部操作记录失败: %w", err)
	}
	result := &OpRecordListResult{
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

// PageRecent 分页读取记录行（最新调用在前，设置页只读列表的读取面）
func (s *OpRecordService) PageRecent(ctx context.Context, pageNumber, pageSize int) (*model.Page[entity.ExternalApiOpRecord], error) {
	return s.repo.PageRecent(ctx, pageNumber, pageSize)
}

// Rotate 轮转：按 TTL 与容量裁最旧（Record 顺带触发），返回裁剪行数
func (s *OpRecordService) Rotate(ctx context.Context) (int64, error) {
	cutoff := util.GetCurrentTimestamp() - opRecordRotationTTL.Milliseconds()
	deleted, err := s.repo.RotateOldest(ctx, cutoff, opRecordRotationMaxRows)
	if err != nil {
		return 0, fmt.Errorf("轮转裁剪外部操作记录失败: %w", err)
	}
	if deleted > 0 {
		logger.Log.Infow("外部操作记录轮转完成", "deleted", deleted)
	}
	return deleted, nil
}

// SetOpRecordStore 装配外部操作记录面（装配期调用，进入运行期后不再更换）。不装配则
// 外部调用不落记录、记录查询端点回「未接入」——记录是问责账，缺它只损失可追溯性，
// 不影响操作面可用性
func (s *Server) SetOpRecordStore(store OpRecordStore) {
	s.opRecords = store
}

// recordInvocation 落一行外部操作记录（操作分发层统一挂点：统一操作端点与查询参数
// 形态的操作端点都在操作完成后调用）。尽力而为——记录面未装配时跳过，写账失败仅日志，
// 不改变操作响应语义。参数摘要写入即脱敏，见 summarizeOperationParams
func (s *Server) recordInvocation(ctx context.Context, operationID string, body []byte, success bool, clientLabel string, took time.Duration) {
	if s.opRecords == nil {
		return
	}
	err := s.opRecords.Record(ctx, operationID, summarizeOperationParams(body), success, clientLabel, took.Milliseconds())
	if err != nil {
		logger.Log.Warnw("外部操作记录写入失败（不影响操作响应）", "operation", operationID, "error", err)
	}
}

// handleOplog 外部操作记录增量查询端点：查询参数 cursor/limit，返回游标契约字段
// （Entries/NextCursor/HasMore/CursorExpired）。与统一操作端点同一信封语义与并发上限；
// 查询参数非法属查询失败：200 + 纯失败信封。记录面未装配时 503 快速失败（不以空结果
// 冒充成功）
func (s *Server) handleOplog(w http.ResponseWriter, r *http.Request) {
	if s.opRecords == nil {
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("外部操作记录未接入"))
		return
	}
	if !s.tryAcquireDispatchSlot() {
		writeJSON(w, http.StatusServiceUnavailable, model.Error[any]("外部接口并发已达上限，请稍后重试"))
		return
	}
	defer s.releaseDispatchSlot()

	var cursor int64
	var limit int
	query := r.URL.Query()
	if raw := query.Get("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusOK, model.Error[any]("cursor 参数非法：需为整数"))
			return
		}
		cursor = parsed
	}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusOK, model.Error[any]("limit 参数非法：需为整数"))
			return
		}
		limit = parsed
	}
	result, err := s.opRecords.ListSince(r.Context(), cursor, limit)
	if err != nil {
		writeJSON(w, http.StatusOK, model.Error[any](err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, model.Success(result))
}

// summarizeOperationParams 构造操作参数的脱敏摘要：请求体 JSON 结构化保留原形，逐字符串
// 值做 URL 脱敏后序列化为紧凑 JSON，超长按字符数截断。非 JSON 请求体不落原文（无法结构
// 化脱敏），只记字节数。摘要仅服务问责回看，不参与任何执行逻辑
func summarizeOperationParams(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Sprintf("（非 JSON 请求体，%d 字节）", len(raw))
	}
	data, err := json.Marshal(maskURLsInValue(value))
	if err != nil {
		return ""
	}
	summary := string(data)
	if utf8.RuneCountInString(summary) > opRecordMaxParamsLength {
		summary = string([]rune(summary)[:opRecordMaxParamsLength]) + "…（截断）"
	}
	return summary
}

// maskURLsInValue 递归遍历解码后的 JSON 值，对每个 URL 形字符串值做脱敏（对象值、数组
// 元素与嵌套结构全走一遍；非字符串值原样保留）
func maskURLsInValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			masked[key] = maskURLsInValue(item)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for i, item := range typed {
			masked[i] = maskURLsInValue(item)
		}
		return masked
	case string:
		return maskURLValue(typed)
	default:
		return value
	}
}

// maskURLValue URL 形字符串的脱敏：仅保留协议+主机+路径，查询串不落库——只记存在与
// 字符数（查询串常携带凭据类参数，进入记录与设置页展示会泄漏）。非 URL 形（无协议或
// 无主机，如普通标识符与 Windows 路径）返回原值
func maskURLValue(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s
	}
	masked := u.Scheme + "://" + u.Host + u.Path
	if u.RawQuery != "" {
		masked += fmt.Sprintf(" ?…（含查询串 %d 字符）", len(u.RawQuery))
	}
	return masked
}
