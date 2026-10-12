package journal

import (
	"context"
	"encoding/json"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// EventTypeWorkIngested 入库域事件类型：作品入库完成（新作品行建成并提交）
const EventTypeWorkIngested = "work_ingested"

// 入库路径来源（载荷 source 字段取值：两获取路径的判别面，供消费方区分来源）
const (
	// IngestSourcePluginDownload 插件下载路径（download 编排经 SaveWorkInfo 建行）
	IngestSourcePluginDownload = "plugin_download"
	// IngestSourceManifestImport manifest 回灌导入路径（分享收件 / zip 导入经 ManifestIngestor 建行）
	IngestSourceManifestImport = "manifest_import"
)

// workIngestedPayload 入库账行的 JSON 摘要载荷（主体引用已在账行 subject_id 列，载荷只补展示面）
type workIngestedPayload struct {
	// WorkID 作品 ID（与账行 subject_id 同值；载荷内冗余一份便于消费方直接读摘要）
	WorkID int64 `json:"workId"`
	// Title 作品标题（work 行 site_work_name 列值，展示用——作品行可能随后被清理，标题留摘要）
	Title string `json:"title"`
	// Source 入库路径来源（见 IngestSource* 常量）
	Source string `json:"source"`
}

// WorkIngestRecorder 入库域记账适配器：两获取路径（插件下载 / manifest 回灌导入）装饰器的
// 共用写账面，一次调用落一行（域=入库域、主体=作品 ID、载荷=JSON 摘要）。写账尽力而为：
// 失败仅日志告警不上抛——业务事务此刻已提交，调用方的返回值与错误语义须保持不变
// （方案定论：提交后写、尽力而为；账未落属风险1 已接受的失败窗口，缺口由 create_time
// 可推导查询兜底）
type WorkIngestRecorder struct {
	svc *Service
}

// NewWorkIngestRecorder 创建入库记账适配器
func NewWorkIngestRecorder(svc *Service) *WorkIngestRecorder {
	return &WorkIngestRecorder{svc: svc}
}

// RecordWorkIngest 单作品入库落账。独立 Background 上下文：落账发生在业务提交之后，不应随
// 调用方任务的取消而取消（对齐任务终态记账的写账上下文语义，task_terminal_adapter.go:119）。
// recorder 或 svc 为 nil（未装配记账服务）时静默跳过
func (r *WorkIngestRecorder) RecordWorkIngest(workID int64, title, source string) {
	if r == nil || r.svc == nil {
		return
	}
	payload, err := json.Marshal(workIngestedPayload{WorkID: workID, Title: title, Source: source})
	if err != nil {
		logger.Log.Errorw("作品入库记账载荷序列化失败（不影响业务返回）",
			"workId", workID, "source", source, "error", err)
		return
	}
	if err := r.svc.Append(context.Background(), entity.JournalDomainIngest, EventTypeWorkIngested, workID, string(payload)); err != nil {
		logger.Log.Errorw("作品入库记账落账失败（不影响业务返回）",
			"workId", workID, "source", source, "error", err)
	}
}

// WorkInfoSaver 作品完整信息保存能力（download 编排对 work.Service 的依赖面签名，见
// download/interfaces.go:42-45）。本包按同签名结构化依赖——避免 journal 反向 import
// download（download 是消费方，装配时装饰器经结构化类型满足其接口）
type WorkInfoSaver interface {
	SaveWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, workResp *sdkdto.WorkResponse) (int64, error)
}

// SaveWorkInfoIngestDecorator SaveWorkInfo 路径（插件下载）的入库记账装饰器：包装原保存器，
// 保存成功（新作品行已在事务内建成并提交，work/service.go:1122-1141 返回作品 ID）后按返回的
// 作品 ID 落一行账；失败原样透传零落账，写账失败不改变返回值与错误语义
type SaveWorkInfoIngestDecorator struct {
	inner    WorkInfoSaver
	recorder *WorkIngestRecorder
}

// NewSaveWorkInfoIngestDecorator 创建 SaveWorkInfo 入库记账装饰器（inner = 原保存器，如 work.Service）
func NewSaveWorkInfoIngestDecorator(inner WorkInfoSaver, recorder *WorkIngestRecorder) *SaveWorkInfoIngestDecorator {
	return &SaveWorkInfoIngestDecorator{inner: inner, recorder: recorder}
}

// 编译期断言：满足 download 编排的注入面签名（结构化类型，零 import 缝合）
var _ WorkInfoSaver = (*SaveWorkInfoIngestDecorator)(nil)

// SaveWorkInfo 透传保存；成功后落账（标题取 WorkResponse.Work.SiteWorkName，判空降级空串）
func (d *SaveWorkInfoIngestDecorator) SaveWorkInfo(ctx context.Context, task *entity.Task, workTask *entity.WorkTask, workResp *sdkdto.WorkResponse) (int64, error) {
	workID, err := d.inner.SaveWorkInfo(ctx, task, workTask, workResp)
	if err != nil {
		return workID, err
	}
	d.recorder.RecordWorkIngest(workID, workTitleFromResponse(workResp), IngestSourcePluginDownload)
	return workID, nil
}

// workTitleFromResponse 作品标题：WorkResponse.Work.SiteWorkName（*string 逐级判空，缺省空串）
func workTitleFromResponse(workResp *sdkdto.WorkResponse) string {
	if workResp == nil || workResp.Work == nil || workResp.Work.SiteWorkName == nil {
		return ""
	}
	return *workResp.Work.SiteWorkName
}

// WorkTitleReader 建行作品标题批量读取（work.Service 实现，GetFullWorkInfoByIds 批量防 N+1）：
// 仅取 WorkFullDTO.Work 的 ID 与标题，供导入路径装配账行载荷
type WorkTitleReader interface {
	GetFullWorkInfoByIds(ctx context.Context, ids []int64) ([]*dto.WorkFullDTO, error)
}

// ManifestIngestJournalDecorator manifest 回灌导入路径（分享收件 / zip 导入，两执行器共用同一
// 注入实例，share/task_execution.go:55,170、import/task_execution.go:154）的入库记账装饰器：
// 实现 importer.ManifestIngestor 包装原实例，Ingest 成功返回后按 ImportResult.CreatedWorkIDs
// 建行清单逐作品落账。清单是阶段3 增补的载荷来源（风险4 核验结论：ImportResult 原本只携带
// 计数与 persistent_store 行清单、不携带建行作品清单，建行作品 ID 此前止步于 ingestor 内部
// workRemap，见 import/ingest.go ingestEntities）。失败原样透传零落账
type ManifestIngestJournalDecorator struct {
	inner    importer.ManifestIngestor
	recorder *WorkIngestRecorder
	titles   WorkTitleReader
}

// NewManifestIngestJournalDecorator 创建 manifest 导入记账装饰器（inner = 原导入器；titles 为
// nil 或读取失败时载荷标题降级空串，主体=作品 ID 已足够消费方回查）
func NewManifestIngestJournalDecorator(inner importer.ManifestIngestor, recorder *WorkIngestRecorder, titles WorkTitleReader) *ManifestIngestJournalDecorator {
	return &ManifestIngestJournalDecorator{inner: inner, recorder: recorder, titles: titles}
}

// 编译期断言：满足两执行器的注入面接口（app.go 装配位替换原实例注入）
var _ importer.ManifestIngestor = (*ManifestIngestJournalDecorator)(nil)

// Ingest 透传导入；成功后按建行清单落账。落账口径=建行：替换/自动增补不新建作品行、不在
// 清单内（与方案「按 ImportResult 建行清单落账」的措辞一致），查重跳过作品同理零落账
func (d *ManifestIngestJournalDecorator) Ingest(ctx context.Context, manifest *export.Manifest, fileSource importer.FileSource, staging importer.IngestStaging, opts *importer.IngestOptions) (*importer.ImportResult, error) {
	result, err := d.inner.Ingest(ctx, manifest, fileSource, staging, opts)
	if err != nil || result == nil || len(result.CreatedWorkIDs) == 0 {
		return result, err
	}
	titleByID := d.loadWorkTitles(result.CreatedWorkIDs)
	for _, workID := range result.CreatedWorkIDs {
		d.recorder.RecordWorkIngest(workID, titleByID[workID], IngestSourceManifestImport)
	}
	return result, nil
}

// loadWorkTitles 批量读取建行作品标题（map 缺项=空标题）。读取失败不阻断记账：载荷降级
// 空标题仅损失展示面，主体引用（作品 ID）不受影响
func (d *ManifestIngestJournalDecorator) loadWorkTitles(ids []int64) map[int64]string {
	titles := make(map[int64]string, len(ids))
	if d.titles == nil {
		return titles
	}
	fulls, err := d.titles.GetFullWorkInfoByIds(context.Background(), ids)
	if err != nil {
		logger.Log.Warnw("入库记账标题批量读取失败（载荷降级空标题）", "count", len(ids), "error", err)
		return titles
	}
	for _, full := range fulls {
		if full == nil || full.Work == nil || full.Work.SiteWorkName == nil {
			continue
		}
		titles[full.Work.Id] = *full.Work.SiteWorkName
	}
	return titles
}
