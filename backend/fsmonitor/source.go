package fsmonitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/util"
)

// LiveEventSource 运行期文件变更事件源。
// 三平台均由 fsnotify 封装：Windows(ReadDirectoryChangesW) / Linux(inotify) / macOS(FSEvents)。
// 降级条件：网络驱动器 / inotify watches 耗尽 / 内核限制等导致构造失败 → 不注入，
// 上层退化为仅启动对账。
type LiveEventSource interface {
	// Start 开始监控根目录(递归)，变更经 events 推送；不可恢复错误经 errs 推送后关闭 events。
	Start(ctx context.Context) (events <-chan FileChange, errs <-chan error, err error)
	// Stop 停止监控并释放底层资源。
	Stop()
}

// fsnotifySource 基于 fsnotify 的 LiveEventSource 实现。
// 递归监控：初始化遍历 workDir 加 watch，运行期 Create 新目录时动态补 watch
// (Linux inotify 不递归，需逐目录加 watch；Windows/macOS 原生递归但多加 watch 无害)。
// 配对：fsnotify 不暴露 inotify 的 cookie(关联移动的标识)，本实现忠实推送 Create/Remove，
// 移动配对交由关联层做指纹匹配兜底。
type fsnotifySource struct {
	workDir string
	watcher *fsnotify.Watcher

	events chan FileChange
	errs   chan error

	stopOnce sync.Once
	stopCh   chan struct{}

	mu      sync.Mutex
	watches map[string]bool // 已加 watch 的目录绝对路径(用于动态补 watch 与 Remove 时判目录)
}

// NewFsnotifySource 创建基于 fsnotify 的事件源。
// 构造期递归加 watch；失败返回 error，上层据此不注入该能力。
func NewFsnotifySource(workDir string) (*fsnotifySource, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建 fsnotify watcher 失败: %w", err)
	}
	s := &fsnotifySource{
		workDir: filepath.Clean(workDir),
		watcher: w,
		events:  make(chan FileChange, 256),
		errs:    make(chan error, 4),
		stopCh:  make(chan struct{}),
		watches: make(map[string]bool),
	}
	if err := s.addWatchRecursive(s.workDir); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("递归监控工作目录失败: %w", err)
	}
	return s, nil
}

// Start 启动事件循环 goroutine，返回事件与错误 channel。
func (s *fsnotifySource) Start(ctx context.Context) (<-chan FileChange, <-chan error, error) {
	go s.loop(ctx)
	return s.events, s.errs, nil
}

// Stop 停止事件循环并释放 watcher。
func (s *fsnotifySource) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		_ = s.watcher.Close()
	})
}

// loop 事件循环：消费 fsnotify 事件转 FileChange 推送，致命错误推 errs 后退出。
func (s *fsnotifySource) loop(ctx context.Context) {
	defer close(s.events)
	defer close(s.errs)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case ev, ok := <-s.watcher.Events:
			if !ok {
				// watcher 已关闭，推致命错误触发上层降级
				s.sendErr(errors.New("fsnotify watcher 已关闭"))
				return
			}
			s.handleEvent(ev)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			s.sendErr(err)
		}
	}
}

// handleEvent 转换单个 fsnotify 事件为 FileChange 并推送。
func (s *fsnotifySource) handleEvent(ev fsnotify.Event) {
	rel, ok := s.toRel(ev.Name)
	if !ok {
		return // 工作目录外或路径异常，忽略
	}
	switch {
	case ev.Op&fsnotify.Remove != 0:
		isDir := s.isWatchedDir(ev.Name)
		s.unwatchIfDir(ev.Name)
		s.send(FileChange{Kind: ChangeRemove, Path: rel, IsDir: isDir, DetectedAt: util.GetCurrentTimestamp()})
	case ev.Op&fsnotify.Rename != 0:
		// 改名/移动的旧名腿：旧路径文件已消失。Windows 同目录改名只发 Rename+Create 不发 Remove
		// （行为锚定 source_rename_probe_test.go），不转发则该消失对消费 Remove 的域不可见
		isDir := s.isWatchedDir(ev.Name)
		s.unwatchIfDir(ev.Name)
		s.send(FileChange{Kind: ChangeRemove, Path: rel, IsDir: isDir, FromRename: true, DetectedAt: util.GetCurrentTimestamp()})
	case ev.Op&fsnotify.Create != 0:
		isDir := s.isDirAtCreate(ev.Name)
		if isDir {
			// 新建目录递归补 watch
			if err := s.addWatchRecursive(ev.Name); err != nil {
				logger.Log.Warnf("[fsmonitor] 动态补 watch 失败 %s: %v", ev.Name, err)
			}
		}
		s.send(FileChange{Kind: ChangeCreate, Path: rel, IsDir: isDir, DetectedAt: util.GetCurrentTimestamp()})
	default:
		// Write/Chmod 不处理：内容修改不在监控范围（外部改写文件内容不产生记录级变更）
	}
}

// send 非阻塞推送事件：channel 满则丢弃并告警，避免事件循环阻塞(丢弃由对账兜底)。
func (s *fsnotifySource) send(fc FileChange) {
	select {
	case s.events <- fc:
	case <-s.stopCh:
	default:
		logger.Log.Warnf("[fsmonitor] 事件 channel 满，丢弃变更: %+v", fc)
	}
}

// sendErr 非阻塞推送错误。
func (s *fsnotifySource) sendErr(err error) {
	select {
	case s.errs <- err:
	case <-s.stopCh:
	default:
	}
}

// addWatchRecursive 递归为 root 及其所有子目录加 watch。
// 单个目录加 watch 失败仅告警不中断(降级：部分目录无 watch)。
func (s *fsnotifySource) addWatchRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if err := s.watcher.Add(path); err != nil {
			logger.Log.Warnf("[fsmonitor] 加 watch 失败 %s: %v", path, err)
			return nil
		}
		s.mu.Lock()
		s.watches[path] = true
		s.mu.Unlock()
		return nil
	})
}

// isDirAtCreate Create 事件到达时判断是否目录(文件刚创建，stat 可能有 race，失败按文件处理)。
func (s *fsnotifySource) isDirAtCreate(absPath string) bool {
	info, err := os.Stat(absPath)
	return err == nil && info.IsDir()
}

// isWatchedDir 判断路径是否已加 watch(已加 watch 的均为目录)。
func (s *fsnotifySource) isWatchedDir(absPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watches[filepath.Clean(absPath)]
}

// unwatchIfDir 若是已监控目录则移除 watch 并从 watches 移除。
func (s *fsnotifySource) unwatchIfDir(absPath string) {
	cleaned := filepath.Clean(absPath)
	s.mu.Lock()
	_, isDir := s.watches[cleaned]
	if isDir {
		delete(s.watches, cleaned)
	}
	s.mu.Unlock()
	if isDir {
		_ = s.watcher.Remove(cleaned)
	}
}

// toRel 绝对路径转相对 workDir 的正斜杠路径；工作目录外或越界返回 ok=false。
func (s *fsnotifySource) toRel(absPath string) (string, bool) {
	cleaned := filepath.Clean(absPath)
	rel, err := filepath.Rel(s.workDir, cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
