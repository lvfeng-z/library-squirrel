package share

// share-receive 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 task_type 注册进
// Manager 策略表，app.go 装配）：
//   - ReceiveExecution：收件人子任务拉取数据流（读本地共享 manifest → 逐文件暂存续传 → ManifestIngestor 回灌导入）
//
// 回灌前置编排（查重三分类 → 确认 → 逐文件内容判定 → 软删+回滚登记）与子 manifest 构造、
// 作品名净化已提取共享至 import 模块（importer.ReplacePlanner 等，行为保持重构）——本执行器
// 经注入的编排器消费，自身专注网络层（拉取/续传/错误分类）。
//
// 分享方发布不走任务模块（发布直跑经 Service 内受监督 goroutine 驱动，生命周期落
// share_record——见 service.go/record.go）。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
	"github.com/library-squirrel/backend/resource"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/task"
	"github.com/library-squirrel/backend/taskManager"
)

// —— share-receive（收件人拉取）——
//
// 数据流：按任务 id 查 share_task 领域行 → 读本地共享 manifest（Receive 预拉落盘父任务作用域）→
// 过滤本作品子集构造子 manifest → 收件人客户端拨中继逐文件拉取本作品文件至暂存作用域（大小对齐
// 即完成；中断后按暂存大小续传，非中止清理）→ ManifestIngestor 回灌导入子 manifest → 成功清理
// 暂存。拉取中断/分享方离线由任务模型承接：暂停/停止保留暂存，重试/恢复从暂存续传；会话终态
// （撤销/过期/不存在）以用户可读文案置失败。领域行缺失或过时（ManifestID==0）显式 Fail。

// 收件拉取退避参数（瞬态错误：网络/分享方离线/中继限流）
const (
	receiveMaxAttempts  = 4           // 单请求最大尝试次数（1 次初始 + 3 次重试）
	receiveBackoffStart = time.Second // 首次退避
	receiveBackoffMax   = 8 * time.Second
)

// ReceiveExecution share-receive（收件人拉取）任务的执行面策略。
type ReceiveExecution struct {
	svc            *Service                  // 提供 workDir / instanceID / 测试可覆写参数
	shareTaskStore ShareTaskStore            // 收件任务领域行查询（执行参数来源）
	ingestor       importer.ManifestIngestor // 回灌导入能力（与 import handler 同一实例，app.go 装配）
	planner        *importer.ReplacePlanner  // 回灌前置编排（查重三分类→确认→内容判定→软删+登记，import 模块共享能力）
}

// NewReceiveExecution 创建 share-receive 执行面策略（shareTaskStore 为收件任务领域行查询能力；
// checker/replaceOps/mountReader 为回灌前置编排依赖，汇入 importer.ReplacePlanner——异常装配
// 缺项时编排维持全新建/既有跳过旧语义）
func NewReceiveExecution(svc *Service, shareTaskStore ShareTaskStore, ingestor importer.ManifestIngestor,
	checker duplicate.DuplicateChecker, replaceOps resource.ReplaceStoreOps,
	mountReader importer.StoreMountReader) *ReceiveExecution {
	return &ReceiveExecution{svc: svc, shareTaskStore: shareTaskStore, ingestor: ingestor,
		planner: importer.NewReplacePlanner(checker, replaceOps, mountReader)}
}

// Execute 收件人子任务拉取主体：读本地共享 manifest → 过滤本作品子集 → 拉取本作品文件至
// 暂存 → 回灌导入 → 清理暂存并置成功；失败置用户可读文案（暂存保留供重试续传）；
// ctx 取消（暂停/停止）不上报终态交控制面接管。领域行缺失或 ManifestID==0（过时载荷）显式 Fail。
func (e *ReceiveExecution) Execute(h taskManager.StrategyHandle) {
	taskID := h.Task().GetID()
	st, err := e.shareTaskStore.GetById(h.RunCtx(), taskID)
	if err != nil || st == nil {
		// 领域行缺失（建树链崩溃窗口遗留）：按过时载荷显式 Fail
		h.Fail("请删除本任务后重新接收分享")
		return
	}
	workDir := e.svc.workDir()
	if workDir == "" {
		settings.NotifyWorkDirUnconfigured("share")
		h.Fail("工作目录未配置，无法接收分享")
		return
	}
	if st.ManifestID == 0 {
		// 过时载荷（存量整体任务）：新代码不兼容存量，不做迁移或降级
		h.Fail("请删除本任务后重新接收分享")
		return
	}
	// 读本地共享 manifest（Receive 预拉落盘父任务目录，子任务不重复网络拉取）
	manifest, err := readSharedManifest(workDir, st.ManifestPath)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		h.Fail(fmt.Sprintf("共享 manifest 版本不支持: %d", manifest.SchemaVersion))
		return
	}
	// 构造只含本作品的子 manifest（按 ManifestID 定位本作品，查重/暂存/导入均收窄到本作品；
	// 共享能力实现在 import 模块）
	sub, err := importer.BuildSubManifest(manifest, st.ManifestID)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	logger.Log.Infof("[share-recv] 任务 %d 执行开始 manifest=%s", taskID, st.ManifestPath)
	// 拨号端点经 relay_host（链接 host 形态）按判定表重判复原（公网 TLS / 豁免网段明文）
	conn, err := receiveConnParamsFromTaskRow(st)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	client, err := newReceiveClient(conn, e.svc.instanceID, e.svc.opts)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	client.taskID = taskID
	ctx := h.RunCtx()
	// 子任务暂存作用域：收件暂存根下按任务 ID（{workDir}/staging/share-receive/{taskID}/，父任务
	// 作用域含共享 manifest.json，与各子任务作用域平级）；恢复场景复用既有作用域
	staging, err := task.EnsureReceiveScope(ctx, workDir, taskID)
	if err != nil {
		h.Fail(fmt.Sprintf("创建暂存目录失败: %v", err))
		return
	}

	// 查重 → 确认 → 逐文件内容判定 → 软删 + 回滚登记（作用域为本作品子集；共享编排能力在
	// import 模块，时序语义同整体路径，见设计七）
	planStart := time.Now()
	plan, canceled, err := e.planner.PlanReplace(ctx, sub, staging, workDir, h)
	logger.Log.Infof("[share-recv] 任务 %d 查重+确认+内容判定+软删 完成 耗时=%s canceled=%v err=%v", taskID, time.Since(planStart), canceled, err)
	if err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	if canceled {
		return // 确认被取消（暂停/停止防御性打断）：不上报终态，交控制面接管
	}
	if ctx.Err() != nil {
		logger.Log.Infof("[share-recv] 任务 %d 查重替换阶段被暂停/停止打断", taskID)
		return // 软删窗口内暂停/停止：回滚清单已登记（交 setFailed 单点），暂停延续替换
	}

	// 阶段二：逐文件拉取至暂存（只拉本作品引用文件；被裁决跳过作品的文件不拉）
	stageStart := time.Now()
	if err := e.stageFiles(ctx, client, staging, sub, importer.FileSkipSet(sub, plan.SkipWorks), h); err != nil {
		logger.Log.Debugf("[share-recv] 任务 %d 拉取阶段失败 耗时=%s err=%v", taskID, time.Since(stageStart), err)
		reportReceiveError(h, ctx, err)
		return
	}
	logger.Log.Infof("[share-recv] 任务 %d 拉取阶段完成 耗时=%s", taskID, time.Since(stageStart))
	if ctx.Err() != nil {
		logger.Log.Infof("[share-recv] 任务 %d 拉取完成后被暂停/停止打断", taskID)
		return
	}

	// 阶段三：回灌导入子 manifest（文件源读暂存；入库/查重/落盘全链复用导出回灌能力）。
	// 替换选项：确认替换与零交集并入注入；二者皆空（无命中替换）则保持全跳过旧语义
	var opts *importer.IngestOptions
	if len(plan.ConfirmedWorks) > 0 || len(plan.AutoMergeWorks) > 0 {
		opts = &importer.IngestOptions{
			ReplaceWorks:   plan.ConfirmedWorks,
			AutoMergeWorks: plan.AutoMergeWorks,
		}
	}
	ingestStart := time.Now()
	imported, err := e.ingestor.Ingest(ctx, sub, stagedFileSource(staging), receiveStaging{workDir: workDir, staging: staging}, opts)
	if err != nil {
		logger.Log.Debugf("[share-recv] 任务 %d 导入失败 耗时=%s err=%v", taskID, time.Since(ingestStart), err)
		reportReceiveError(h, ctx, err)
		return
	}
	// 导入建行事务已提交：新建 store 行清单并入终态回滚登记——导入成功到 Finish 之间的
	// 停止/失败窗口内，控制面复活被软删受害者前先物理丢弃新建行（释放其占用的 file_path，
	// 避免新旧两代同路径并存拒绝复活）
	if len(imported.CreatedStoreIDs) > 0 {
		h.SetTerminalRollback(taskManager.TerminalRollback{CreatedStoreIDs: imported.CreatedStoreIDs})
	}
	logger.Log.Infof("[share-recv] 任务 %d 导入完成 耗时=%s", taskID, time.Since(ingestStart))
	// 成功：清理本任务暂存（共享 manifest.json 在父任务目录，不动；残留由启动清扫回收）
	_ = os.RemoveAll(staging)
	h.Finish()
	logger.Log.Infof("[share-recv] 任务 %d 执行完成", taskID)
}

// readSharedManifest 读本地共享 manifest：workDir 相对路径（正斜杠 relPath 域），
// 在 os.ReadFile 调用点现场 join 为绝对路径（absPath 域）。
func readSharedManifest(workDir, relPath string) (*export.Manifest, error) {
	if relPath == "" {
		return nil, errors.New("收件任务缺少共享清单路径")
	}
	data, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		return nil, fmt.Errorf("读取共享 manifest 失败: %w", err)
	}
	manifest, err := export.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("解析共享 manifest 失败: %w", err)
	}
	return manifest, nil
}

// reportReceiveError 统一的错误收口：ctx 已取消（暂停/停止）不上报终态交控制面接管，
// 其余按分类转用户可读文案置失败。
func reportReceiveError(h taskManager.StrategyHandle, ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	h.Fail(receiveUserMessage(err))
}

// receiveUserMessage 错误 → 用户可读文案（中继拨号分类/流内错误/截断/透传）
func receiveUserMessage(err error) string {
	var de *relayDialError
	if errors.As(err, &de) {
		if msg, _ := relayDialFailMessage(err); msg != "" {
			return msg
		}
		return de.Error()
	}
	var ae *streamAppError
	if errors.As(err, &ae) {
		switch ae.code {
		case streamErrNotFound:
			return "分享方数据与清单不一致，无法完成拉取"
		case streamErrMissing:
			return "分享方源文件缺失，无法完成拉取"
		case streamErrBadRequest:
			return "拉取请求被分享方拒绝"
		default:
			return fmt.Sprintf("分享方内部错误（%s）", ae.code)
		}
	}
	if errors.Is(err, errStreamTruncated) {
		return "拉取数据不完整（传输中断），可在任务面板重试续传"
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func (e *ReceiveExecution) stageFiles(ctx context.Context, client *receiveClient, staging string,
	manifest *export.Manifest, skipFiles map[int64]struct{}, h taskManager.StrategyHandle) error {
	taskID := h.Task().GetID()
	// 待拉条目判定：缺包内路径/源缺失标记/被裁决跳过作品的文件 一律跳过
	pull := func(entry *export.FileEntry) bool {
		if entry.Path == "" || entry.Missing {
			return false
		}
		_, skip := skipFiles[entry.StoreID]
		return !skip
	}
	// 进度基数：全部待拉条目的声明字节（已完成条目也计入基数，finished 经暂存盘点补齐）
	var total int64
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		total += entry.Size
	}
	var done int64
	report := func() {
		if total > 0 {
			h.ReportProgress(total, done)
			logger.Log.Debugf("[share-recv] 任务 %d 进度推进 total=%d done=%d", taskID, total, done)
		}
	}
	// 初始进度：已暂存字节（重试/恢复任务的即时段位）
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		if sz := stagedSize(importer.StagingPath(staging, entry.Path)); sz > 0 && sz <= entry.Size {
			done += sz
		}
	}
	report()
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if !pull(entry) {
			continue
		}
		fileStart := time.Now()
		if err := stageFile(ctx, client, staging, entry, func(delta int64) {
			done += delta
			report()
		}); err != nil {
			var ae *streamAppError
			if errors.As(err, &ae) && ae.code == streamErrMissing {
				// 分享方现报缺失：置清单缺席标记，导入按挂载缺席降级（其余照常）
				logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 分享方现报缺失", taskID, entry.Path)
				entry.Missing = true
				continue
			}
			logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取失败 耗时=%s err=%v", taskID, entry.Path, time.Since(fileStart), err)
			return err
		}
		logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取完成 耗时=%s", taskID, entry.Path, time.Since(fileStart))
	}
	report()
	return nil
}

// stagedSize 暂存文件已落盘字节数（不存在为 0）
func stagedSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// stageFile 拉取单文件至暂存（含瞬态退避重试与断点续传）：
//   - 暂存大小 == 声明大小：跳过（上次已完成）
//   - 0 < 暂存大小 < 声明大小：从该偏移续传（对齐流内协议 file 请求 offset 锚）
//   - 暂存大小异常（> 声明/0 字节残留）：重建（截断重拉）
func stageFile(ctx context.Context, client *receiveClient, staging string, entry *export.FileEntry,
	onProgress func(delta int64)) error {
	target := importer.StagingPath(staging, entry.Path)
	if entry.Size > 0 && stagedSize(target) == entry.Size {
		return nil // 上次已完整拉取
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建暂存子目录失败: %w", err)
	}
	var offset int64
	if sz := stagedSize(target); sz > 0 && sz < entry.Size {
		offset = sz
	} else if sz > entry.Size {
		// 残留超过声明（清单变更过的旧暂存）：截断重拉
		if err := os.Truncate(target, 0); err != nil {
			return fmt.Errorf("重置暂存文件失败: %w", err)
		}
	}
	logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取准备 暂存=%d 声明=%d 续传offset=%d", client.taskID, entry.Path, stagedSize(target), entry.Size, offset)
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		err := appendFetch(ctx, client, entry, target, offset, onProgress)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取被取消(attempt=%d) err=%v", client.taskID, entry.Path, attempt, err)
			return ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return err
		}
		logger.Log.Debugf("[share-recv] 任务 %d 文件 %s 拉取瞬态错误重试 attempt=%d err=%v 退避=%s", client.taskID, entry.Path, attempt, err, delay)
		// 重试前按实际落盘字节重算续传锚（appendFetch 失败点即上次落盘点）
		offset = stagedSize(target)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < receiveBackoffMax {
			delay *= 2
		}
	}
}

// appendFetch 一次 file 请求的拉取落盘：从 offset 起追加写暂存文件，校验应答头声明与
// 请求锚一致、终态字节数与声明一致。
func appendFetch(ctx context.Context, client *receiveClient, entry *export.FileEntry,
	target string, offset int64, onProgress func(delta int64)) error {
	_, err := fetchWithRetry(ctx, client, &streamRequest{Type: "file", Path: entry.Path, Offset: offset},
		func(head *streamHeader, r *streamContentReader) (bool, error) {
			if head.Kind != "file" || head.Offset != offset || head.Size != entry.Size-offset {
				return false, fmt.Errorf("%w：应答头与请求不匹配（offset=%d size=%d，期望 offset=%d size=%d）",
					errStreamTruncated, head.Offset, head.Size, offset, entry.Size-offset)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return false, fmt.Errorf("打开暂存文件失败: %w", err)
			}
			defer func() { _ = f.Close() }()
			buf := make([]byte, 32*1024)
			var written int64
			for {
				n, rerr := r.Read(buf)
				if n > 0 {
					if _, werr := f.Write(buf[:n]); werr != nil {
						return false, fmt.Errorf("写暂存文件失败: %w", werr)
					}
					written += int64(n)
					onProgress(int64(n))
				}
				if rerr == io.EOF {
					if written != head.Size {
						return false, fmt.Errorf("%w：实收 %d 字节，声明 %d", errStreamTruncated, written, head.Size)
					}
					return true, nil
				}
				if rerr != nil {
					return false, rerr
				}
			}
		})
	return err
}

// readAllBody 应答体全量读入（manifest 拉取用）
func readAllBody(head *streamHeader, r *streamContentReader) (body []byte, err error) {
	defer func() { _ = r.Close() }()
	if head.Kind != "manifest" {
		return nil, fmt.Errorf("manifest 应答 kind 异常: %s", head.Kind)
	}
	body, err = io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if head.Size > 0 && int64(len(body)) != head.Size {
		return nil, errStreamTruncated
	}
	return body, nil
}

// fetchWithRetry 单请求瞬态退避重试壳：非瞬态错误（中继终态拒绝/流内应用错误）与重试
// 耗尽原样上抛；body 消费中途失败同样按瞬态重试（文件场景由暂存大小自然续传）。
func fetchWithRetry[T any](ctx context.Context, client *receiveClient, req *streamRequest,
	body func(head *streamHeader, r *streamContentReader) (T, error)) (T, error) {
	var zero T
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		fetchStart := time.Now()
		head, r, err := client.fetch(ctx, req)
		logger.Log.Debugf("[share-recv] task=%d fetch 尝试#%d path=%s 拨号+应答耗%s err=%v", client.taskID, attempt, req.Path, time.Since(fetchStart), err)
		if err == nil {
			res, berr := body(head, r)
			partial := r.got > 0
			_ = r.Close()
			if berr == nil {
				return res, nil
			}
			err = berr
			logger.Log.Debugf("[share-recv] task=%d fetch path=%s 内容消费失败 got=%d err=%v", client.taskID, req.Path, r.got, err)
			// 内容已部分消费后失败：本次尝试已把部分字节写入暂存（O_APPEND 追加），同 offset
			// 重试会叠加重复字节——文件超长损坏且进度双计（实测：传输中中断即现）。交外层
			// 续传锚重算（stageFile 按实际落盘字节重设 offset）处理，不再内层重复追加。
			if partial {
				return zero, err
			}
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			logger.Log.Debugf("[share-recv] task=%d fetch path=%s 判定终态错误 attempt=%d err=%v", client.taskID, req.Path, attempt, err)
			return zero, err
		}
		logger.Log.Debugf("[share-recv] task=%d fetch path=%s 重试退避 attempt=%d delay=%s err=%v", client.taskID, req.Path, attempt, delay, err)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
		if delay < receiveBackoffMax {
			delay *= 2
		}
	}
}

// stagedFileSource 构建暂存目录 → 导入文件源（FileSource 的暂存实现）：包内路径做
// 白名单校验（禁绝对路径/反斜杠/穿越段），仅允许读暂存内既有文件。
func stagedFileSource(staging string) importer.FileSource {
	return func(entryPath string) (io.ReadCloser, error) {
		if !safeEntryPath(entryPath) {
			return nil, fmt.Errorf("包内路径不合法: %s", entryPath)
		}
		f, err := os.Open(importer.StagingPath(staging, entryPath))
		if err != nil {
			return nil, fmt.Errorf("%w：%s", importer.ErrPackageFileMissing, entryPath)
		}
		return f, nil
	}
}

// receiveStaging 收件暂存轨的导入暂存层（importer.IngestStaging 实现）：文件已由拉取阶段落
// {workDir}/staging/share-receive/{taskID}/ 可续传作用域（暂存大小对齐声明即跳过重拉、不足按
// 偏移续传，见 stageFile），直接作落位来源——解包相位不写暂存（writer=nil）、仅读流实测 sha；
// 撤回处置=退回暂存（落位后失败/中断的字节回到可续传位置，任务重试免网络重拉）；导入退出
// 不回收作用域（生命周期归任务：成功由执行面清理、暂停/失败保留续传、任务删除链与启动清扫
// 兜底）。
type receiveStaging struct {
	workDir string // 库根（登记行暂存路径的派生基准）
	staging string // 收件作用域绝对路径（absPath 域）
}

// Stage 内容已在收件作用域：不写暂存（writer=nil），登记行暂存路径=作用域内条目镜像路径。
func (s receiveStaging) Stage(ctx context.Context, entryPath string) (io.WriteCloser, string, error) {
	rel, err := receiveStagingRel(s.workDir, s.staging, entryPath)
	if err != nil {
		return nil, "", err
	}
	return nil, rel, nil
}

// AbortAction 撤回处置声明：退回暂存（收件文件按暂存大小偏移续传，字节保留即免重拉）。
func (s receiveStaging) AbortAction() entity.IngestAbortAction {
	return entity.AbortActionReturnToStaging
}

// Release 导入退出收尾：收件作用域生命周期归任务，导入不回收（空操作）。
func (s receiveStaging) Release(ctx context.Context) {}

// receiveStagingRel 收件作用域内条目的 workDir 相对路径（relPath 域正斜杠）：作用域绝对路径
// 按 workDir 前缀剥离还原（跨域收参边界 ToSlash 规范化），前缀不匹配（路径派生异常）显式
// 报错不兜底。
func receiveStagingRel(workDir, stagingAbs, entryPath string) (string, error) {
	absSlash := filepath.ToSlash(stagingAbs)
	rootSlash := strings.TrimSuffix(filepath.ToSlash(workDir), "/")
	if workDir == "" || !strings.HasPrefix(absSlash, rootSlash+"/") {
		return "", fmt.Errorf("收件暂存路径未落在工作目录下(%s)", stagingAbs)
	}
	return path.Join(strings.TrimPrefix(absSlash, rootSlash+"/"), entryPath), nil
}

// safeEntryPath 包内路径白名单校验：非空、正斜杠相对路径、无穿越段
func safeEntryPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
