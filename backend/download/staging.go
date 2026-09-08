package download

// 下载暂存基建（执行面侧）：暂存文件写入器（download 直接管理文件与全量 sha256 流式哈希）
// 与运行中任务的暂存规划注册面（GetStoreRelPath 运行形态查询的数据源）。
// 暂存模式下下载内容先写 {workDir}/task-staging/{taskID}/ 下的 role_seq 键文件，
// 全部轨道写满后由提交点统一 rename 进 store/ 最终路径——暂存期内长下载全程零 DB 副作用；
// task-staging/ 不在 store/ 白名单子树内，fsmonitor 对其零感知（无需抑制登记）。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// stagingWriter 暂存文件写入器：download 直接管理的暂存文件句柄 + 写入流全量 sha256 哈希器。
// 哈希与文件写入同步累计（写入流形态的全量哈希，非事后按路径计算）；续传打开时先把已落盘
// 前缀读入哈希器，保证跨会话续写的实测哈希与文件内容一致。
// 生命周期：写入中 Write/ Sync；暂停 Close（文件保留供续传）；写满 finalize（Sync+Close+比对
// 来源声明的期望哈希，不符即错误返回——暂存保留供诊断，重试重下覆盖）。
type stagingWriter struct {
	file     *os.File
	hasher   hash.Hash // 全量 sha256（每次写入同步喂入）
	expected string    // 来源声明的期望 SHA256（hex；空=未声明，跳过比对）
	closed   bool
}

// newStagingWriterFresh 全新打开暂存文件（写入从 0 起，既有文件截断——失败残留/旧轮次覆盖）
func newStagingWriterFresh(absPath string, expected string) (*stagingWriter, error) {
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("创建暂存文件失败: %w", err)
	}
	return &stagingWriter{file: file, hasher: sha256.New(), expected: expected}, nil
}

// newStagingWriterResume 续传打开：截断到 writeOffset（消除 stat 与打开间的 TOCTOU 多余数据）
// 并把 [0, writeOffset) 前缀读入哈希器。writeOffset 为 0 时等价于全新打开（保留既有语义边界）
func newStagingWriterResume(absPath string, writeOffset int64, expected string) (*stagingWriter, error) {
	if writeOffset <= 0 {
		return newStagingWriterFresh(absPath, expected)
	}
	file, err := os.OpenFile(absPath, os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开暂存文件失败: %w", err)
	}
	if err := file.Truncate(writeOffset); err != nil {
		file.Close()
		return nil, fmt.Errorf("截断暂存文件到偏移 %d 失败: %w", writeOffset, err)
	}
	hasher := sha256.New()
	if rd, rerr := os.Open(absPath); rerr == nil {
		_, _ = io.CopyN(hasher, rd, writeOffset)
		_ = rd.Close()
	} else {
		file.Close()
		return nil, fmt.Errorf("读取暂存前缀失败: %w", rerr)
	}
	if _, err := file.Seek(writeOffset, io.SeekStart); err != nil {
		file.Close()
		return nil, fmt.Errorf("定位暂存写入偏移 %d 失败: %w", writeOffset, err)
	}
	return &stagingWriter{file: file, hasher: hasher, expected: expected}, nil
}

func (w *stagingWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if n > 0 {
		// 哈希器不返回错误（sha256 写入恒成功），按实际落盘字节喂入
		_, _ = w.hasher.Write(p[:n])
	}
	return n, err
}

func (w *stagingWriter) Sync() error {
	if w.closed {
		return nil
	}
	return w.file.Sync()
}

func (w *stagingWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

// finalize 暂存写满收尾：Sync + Close 后比对完整性哈希。期望未声明（空）跳过比对；
// 不符返回错误（调用方按任务失败收口，暂存文件保留供诊断与重试覆盖）。
// 返回实测哈希 hex（供提交点建行落 actual_sha256 列）
func (w *stagingWriter) finalize() (actualSha string, err error) {
	if !w.closed {
		if serr := w.file.Sync(); serr != nil {
			_ = w.file.Close()
			w.closed = true
			return "", fmt.Errorf("同步暂存文件失败: %w", serr)
		}
		if cerr := w.file.Close(); cerr != nil {
			w.closed = true
			return "", fmt.Errorf("关闭暂存文件失败: %w", cerr)
		}
		w.closed = true
	}
	actual := hex.EncodeToString(w.hasher.Sum(nil))
	if w.expected != "" && !strings.EqualFold(w.expected, actual) {
		return actual, fmt.Errorf("来源声明的哈希与下载内容不符（期望 %s，实测 %s）", w.expected, actual)
	}
	return actual, nil
}

// StagingPlanner 运行中任务的暂存规划注册面：taskId → (role, store_seq) → 最终 relPath。
// 执行面在 Start/Resume 返回 specs 解析出全部最终路径后注册，执行结束（终态/中断返回）注销；
// GetStoreRelPath 运行形态查询先查本表（插件 document lazy 生成要的是最终文件名，
// 文件物理在暂存但契约解耦）。map+mutex，注册/注销/查询均 O(1)
type StagingPlanner struct {
	mu    sync.Mutex
	plans map[int64]map[storeIdentity]string
}

// NewStagingPlanner 构建暂存规划注册面（装配层单例注入执行面与 GetStoreRelPath 适配器）
func NewStagingPlanner() *StagingPlanner {
	return &StagingPlanner{plans: make(map[int64]map[storeIdentity]string)}
}

// Register 注册任务的全部轨道最终路径（同任务重复注册整体覆盖——单 actor 串行执行，无并发覆盖）
func (p *StagingPlanner) Register(taskId int64, finals map[storeIdentity]string) {
	if len(finals) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plans[taskId] = finals
}

// Unregister 注销任务的规划（执行结束调用；后续查询回落到已提交行直查）
func (p *StagingPlanner) Unregister(taskId int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.plans, taskId)
}

// FinalRelPath 运行形态查询：命中返回最终 relPath 与 true；未注册/未命中返回 false
func (p *StagingPlanner) FinalRelPath(taskId int64, role string, storeSeq int) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	plan, ok := p.plans[taskId]
	if !ok {
		return "", false
	}
	rel, ok := plan[storeIdentity{role: role, seq: storeSeq}]
	return rel, ok
}

// storeIdentity store 轨道身份键:同 role 内 store_seq 唯一定位一个 store(N-同 role 多 store
// 支持)。规划表键与暂存文件名键同维度（role_seq 三位零填充即本键的文件名形态）
type storeIdentity struct {
	role string
	seq  int
}

// stagingEntry 暂存目录枚举出的单轨条目（role_seq 文件名还原身份 + 已落盘字节数）
type stagingEntry struct {
	role string
	seq  int
	size int64
}

// enumerateStaging 枚举任务暂存目录的全部轨道：文件名按 role_seq 键解析身份，os.Stat 得
// 各轨偏移（暂停/崩溃后恢复的续传锚来源）。无法解析身份的文件跳过（人工诊断残留）
func enumerateStaging(stagingDir string) ([]stagingEntry, error) {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]stagingEntry, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		role, seq, perr := parseStagingFileName(ent.Name())
		if perr != nil {
			continue
		}
		info, serr := ent.Info()
		if serr != nil {
			continue
		}
		out = append(out, stagingEntry{role: role, seq: seq, size: info.Size()})
	}
	// ReadDir 已按名排序；再按 (role, seq) 稳定排序，保证偏移列表顺序确定（spec 配对依赖）
	sort.Slice(out, func(i, j int) bool {
		if out[i].role != out[j].role {
			return out[i].role < out[j].role
		}
		return out[i].seq < out[j].seq
	})
	return out, nil
}

// parseStagingFileName 解析暂存文件名 {role}_{seq 三位零填充}{ext} 还原 (role, seq)。
// store_type 封闭枚举值不含下划线，取最后一个下划线分段为 seq（StagingFileName 的逆）
func parseStagingFileName(name string) (role string, seq int, err error) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	idx := strings.LastIndex(base, "_")
	if idx <= 0 || idx == len(base)-1 {
		return "", 0, fmt.Errorf("暂存文件名 %q 不含 role_seq 键", name)
	}
	seq, serr := strconv.Atoi(base[idx+1:])
	if serr != nil || seq < 0 {
		return "", 0, fmt.Errorf("暂存文件名 %q 的 seq 段无法解析", name)
	}
	return base[:idx], seq, nil
}
