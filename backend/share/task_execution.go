package share

// share-receive 任务的执行面策略（taskManager.ExecutionStrategy 实现，按 task_type 注册进
// Manager 策略表，app.go 装配）：
//   - ReceiveExecution：收件人拉取数据流（拉 manifest → 逐文件暂存续传 → ManifestIngestor 回灌导入）
//
// 分享方发布不走任务模块（发布直跑经 Service 内受监督 goroutine 驱动，生命周期落
// share_record——见 service.go/record.go）。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/library-squirrel/backend/export"
	importer "github.com/library-squirrel/backend/import"
	"github.com/library-squirrel/backend/taskManager"
)

// —— share-receive（收件人拉取）——
//
// 数据流：反解载荷 → 收件人客户端拨中继 → 拉 manifest → 逐文件拉取至暂存目录
// （大小对齐即完成；中断后按暂存大小续传，非中止清理）→ ManifestIngestor 回灌导入 →
// 成功清理暂存。拉取中断/分享方离线由任务模型承接：暂停/停止保留暂存，重试/恢复从
// 暂存续传；会话终态（撤销/过期/不存在）以用户可读文案置失败。

// receiveStagingRootName workDir 下的收件暂存目录名（任务行一个子目录；不在 store/ 白名单
// 子树内，fsmonitor 不感知）
const receiveStagingRootName = "share-receive"

// 收件拉取退避参数（瞬态错误：网络/分享方离线/中继限流）
const (
	receiveMaxAttempts  = 4           // 单请求最大尝试次数（1 次初始 + 3 次重试）
	receiveBackoffStart = time.Second // 首次退避
	receiveBackoffMax   = 8 * time.Second
)

// ReceiveExecution share-receive（收件人拉取）任务的执行面策略。
type ReceiveExecution struct {
	svc      *Service                  // 提供 workDir / instanceID / 测试可覆写参数
	ingestor importer.ManifestIngestor // 回灌导入能力（与 import handler 同一实例，app.go 装配）
}

// NewReceiveExecution 创建 share-receive 执行面策略
func NewReceiveExecution(svc *Service, ingestor importer.ManifestIngestor) *ReceiveExecution {
	return &ReceiveExecution{svc: svc, ingestor: ingestor}
}

// Execute 收件人拉取主体：拉取至暂存 → 回灌导入 → 清理暂存并置成功；
// 失败置用户可读文案（暂存保留供重试续传）；ctx 取消（暂停/停止）不上报终态交控制面接管。
func (e *ReceiveExecution) Execute(h taskManager.StrategyHandle) {
	task := h.Task()
	payload, err := parseShareReceivePayload(task.Payload.String)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	workDir := e.svc.workDir()
	if workDir == "" {
		h.Fail("工作目录未配置，无法接收分享")
		return
	}
	client, err := newReceiveClient(payload, e.svc.instanceID, e.svc.opts)
	if err != nil {
		h.Fail(err.Error())
		return
	}
	ctx := h.RunCtx()
	staging := filepath.Join(workDir, receiveStagingRootName, strconv.FormatInt(task.GetID(), 10))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		h.Fail(fmt.Sprintf("创建暂存目录失败: %v", err))
		return
	}

	// 阶段一：拉 manifest（中转字节全量小，一次拉全；瞬态错误退避重试）
	manifestData, err := fetchWithRetry(ctx, client, &streamRequest{Type: "manifest"}, readAllBody)
	if err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	manifest, err := export.Deserialize(manifestData)
	if err != nil {
		h.Fail(fmt.Sprintf("解析 manifest 失败: %v", err))
		return
	}

	// 阶段二：逐文件拉取至暂存（断点续传锚 = 暂存已落盘字节数）
	if err := e.stageFiles(ctx, client, staging, manifest, h); err != nil {
		reportReceiveError(h, ctx, err)
		return
	}

	// 阶段三：回灌导入（文件源读暂存；入库/查重/落盘全链复用导出回灌能力）
	if _, err := e.ingestor.Ingest(ctx, manifest, stagedFileSource(staging)); err != nil {
		reportReceiveError(h, ctx, err)
		return
	}
	// 成功：清理暂存（任务行保留，删除任务行由启动清扫兜底回收）；清理失败不置失败终态
	// （导入已成功），残留由启动清扫回收
	_ = os.RemoveAll(staging)
	h.Finish()
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

// stageFiles 逐文件拉取暂存：已完成（暂存大小==声明大小）跳过；部分完成按暂存大小续传；
// 分享方现报缺失（发布后源文件被删）的条目标记 Missing 走导入缺席降级（对齐决策4）。
func (e *ReceiveExecution) stageFiles(ctx context.Context, client *receiveClient, staging string,
	manifest *export.Manifest, h taskManager.StrategyHandle) error {
	// 进度基数：全部待拉条目的声明字节（已完成条目也计入基数，finished 经暂存盘点补齐）
	var total int64
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		total += entry.Size
	}
	var done int64
	report := func() {
		if total > 0 {
			h.ReportProgress(total, done)
		}
	}
	// 初始进度：已暂存字节（重试/恢复任务的即时段位）
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		if sz := stagedSize(stagingPath(staging, entry.Path)); sz > 0 && sz <= entry.Size {
			done += sz
		}
	}
	report()
	for i := range manifest.Files {
		entry := &manifest.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		if err := stageFile(ctx, client, staging, entry, func(delta int64) {
			done += delta
			report()
		}); err != nil {
			var ae *streamAppError
			if errors.As(err, &ae) && ae.code == streamErrMissing {
				// 分享方现报缺失：置清单缺席标记，导入按挂载缺席降级（其余照常）
				entry.Missing = true
				continue
			}
			return err
		}
	}
	report()
	return nil
}

// stagingPath 暂存内绝对路径（包内路径正斜杠 → 平台分隔符；absPath 域，仅 os 调用点使用）
func stagingPath(staging, entryPath string) string {
	return filepath.Join(staging, filepath.FromSlash(entryPath))
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
	target := stagingPath(staging, entry.Path)
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
	delay := receiveBackoffStart
	for attempt := 0; ; attempt++ {
		err := appendFetch(ctx, client, entry, target, offset, onProgress)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return err
		}
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
		head, r, err := client.fetch(ctx, req)
		if err == nil {
			res, berr := body(head, r)
			_ = r.Close()
			if berr == nil {
				return res, nil
			}
			err = berr
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !isRelayRetryableErr(err) || attempt >= receiveMaxAttempts-1 {
			return zero, err
		}
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
		f, err := os.Open(stagingPath(staging, entryPath))
		if err != nil {
			return nil, fmt.Errorf("%w：%s", importer.ErrPackageFileMissing, entryPath)
		}
		return f, nil
	}
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

// CleanupOrphanReceiveStaging 启动清扫：回收任务行已不存在的收件暂存目录（任务删除后
// 其暂存随之失去归属；成功任务的暂存已在执行尾清理，此处兜底崩溃残留与已删任务残留）。
// exists 由调用方提供任务行存在性查询。
func CleanupOrphanReceiveStaging(workDir string, exists func(id int64) bool) error {
	if workDir == "" {
		return nil
	}
	root := filepath.Join(workDir, receiveStagingRootName)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id, perr := strconv.ParseInt(ent.Name(), 10, 64)
		if perr != nil || id <= 0 {
			continue
		}
		if !exists(id) {
			if rerr := os.RemoveAll(filepath.Join(root, ent.Name())); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}
