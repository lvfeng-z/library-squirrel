//go:build windows

// USN Journal 离线追溯的 provider 实现（节点 C 的 C-4）：实现 OfflineChangeProvider，
// 在软件未运行期间基于 NTFS USN Journal 续读精确文件变更（带 rename 配对、不依赖指纹）。
//
// 数据流：FSCTL_QUERY_USN_JOURNAL 取 journal 元信息 → 读 CursorStore 续读起点 →
// FSCTL_READ_USN_JOURNAL 分批续读 → C-1 解析 + C-2 FRN→路径解析 + rename 按 FRN 配对 →
// []FileChange（交 C-6 编排经 Correlator 关联）。
//
// 卷句柄/QUERY/READ 均需管理员（R2）；本 provider 仅在 NewPlatformDeps 检测到 isElevated 时注入。
package fsmonitor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
	"golang.org/x/sys/windows"
)

// ErrCursorInvalid 游标失效：离线期 journal 截断覆盖了续读起点（StartUsn < FirstUsn，R3），
// 或卷格式化致 UsnJournalID 变化（R5，由 CursorStore 的 journal_id 键天然判定为首次）。
// C-6 编排据此降级全量对账；provider 已将游标重置到 NextUsn，下次正常续读。
var ErrCursorInvalid = errors.New("USN 游标失效：续读起点已被 journal 覆盖")

// FSCTL 常量（x/sys/windows 未导出卷级 USN 码，按 winioctl.h CTL_CODE 自算；取自已验证 PoC e9ee2d1）：
//
//	FSCTL_QUERY_USN_JOURNAL = CTL_CODE(9, 61, METHOD_BUFFERED, 0) = 0x000900F4
//	FSCTL_READ_USN_JOURNAL  = CTL_CODE(9, 46, METHOD_NEITHER, 0) = 0x000900BB  ← METHOD_NEITHER（输出须堆缓冲）
const (
	fsctlQueryUSNJournal = 0x000900F4
	fsctlReadUSNJournal  = 0x000900BB

	fsctlShareMode = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

	// readJournalBatchSize 单批 READ_USN_JOURNAL 输出缓冲。METHOD_NEITHER 须堆分配（栈缓冲触发 ERROR_INVALID_USER_BUFFER）。
	readJournalBatchSize = 65536
	// readJournalMaxRecords 单次 ChangesSince 读取记录上限（安全阀，防异常大 journal 撑爆内存；
	// 达上限则存已读进度 lastUsn < NextUsn，下次续读。NTFS 32MB journal 满约 40 万条，上限覆盖之）。
	readJournalMaxRecords = 1 << 19 // 524288
)

// usnPathReasonMask USN 路径类变更 Reason 位掩码（增删改名的路径类，排除数据修改类）。
const usnPathReasonMask = usnReasonFileCreate | usnReasonFileDelete | usnReasonRenameOld | usnReasonRenameNew

// usnJournalData FSCTL_QUERY_USN_JOURNAL 输出（7×uint64，56 字节，自然对齐；栈分配）。
type usnJournalData struct {
	UsnJournalID    uint64
	FirstUsn        int64
	NextUsn         int64
	LowestValidUsn  uint64
	MaxUsn          uint64
	MaximumSize     uint64
	AllocationDelta uint64
}

// readUSNJournalDataV1 FSCTL_READ_USN_JOURNAL 输入（V1，48 字节）。
// 必须用 V1：V0（32 字节）缺 UsnJournalID/版本字段，驱动按 V1 读越界返回 ERROR_INVALID_USER_BUFFER（C-0b 实测）。
type readUSNJournalDataV1 struct {
	StartUsn          int64
	ReasonMask        uint32
	ReturnOnlyOnClose uint32
	Timeout           uint64
	BytesToWaitFor    uint64
	UsnJournalID      uint64
	MinMajorVersion   uint16
	MaxMajorVersion   uint16
}

// usnProvider 基于 USN Journal 的 OfflineChangeProvider 实现。
type usnProvider struct {
	workDir string        // workDir 绝对路径（游标绑定 + 缓存遍历基准）
	volume  string        // 卷设备路径 \\.\E:（从 workDir 推导）
	cursors CursorStore   // USN 游标持久化（C-3），provider 持有（唯一能 QUERY 拿 UsnJournalID 者）
	cache   *frnPathCache // FRN→路径缓存（C-2），懒构建（首次有待读变更时）
}

// NewUsnProvider 创建 USN 离线追溯 provider（Windows）。非 Windows 见 usn_provider_other.go 桩。
func NewUsnProvider(workDir string, cursors CursorStore) OfflineChangeProvider {
	return &usnProvider{
		workDir: workDir,
		volume:  volumeDevicePath(workDir),
		cursors: cursors,
	}
}

// ChangesSince 续读离线期变更。cursor 参数忽略（游标由 provider 经 CursorStore 自管），
// 返回 nil cursor。流程见方案 §四/§五。
func (p *usnProvider) ChangesSince(ctx context.Context, _ OfflineCursor) ([]FileChange, OfflineCursor, error) {
	h, err := openVolume(p.volume)
	if err != nil {
		return nil, nil, err
	}
	defer windows.CloseHandle(h)

	j, err := queryJournal(h)
	if err != nil {
		return nil, nil, err
	}

	cur, err := p.cursors.Get(ctx, j.UsnJournalID, p.workDir)
	if err != nil {
		return nil, nil, fmt.Errorf("读 USN 游标失败: %w", err)
	}

	var startUsn int64
	if cur == nil {
		// 首次启动（或卷变化 R5）：不追溯历史，从 NextUsn 起（D5），存游标
		if err := p.cursors.Save(ctx, Cursor{JournalID: j.UsnJournalID, StartUsn: j.NextUsn, WorkDir: p.workDir}); err != nil {
			return nil, nil, fmt.Errorf("存首次游标失败: %w", err)
		}
		logger.Log.Infof("[fsmonitor] USN 首次启动：游标定于 NextUsn=%d，不追溯历史（离线历史由对账覆盖）", j.NextUsn)
		return nil, nil, nil
	}
	startUsn = cur.StartUsn

	// R3 游标失效：journal 截断覆盖了起点 → 重置游标 + 返回失效错误（C-6 降级对账）
	if startUsn < j.FirstUsn {
		_ = p.cursors.Save(ctx, Cursor{JournalID: j.UsnJournalID, StartUsn: j.NextUsn, WorkDir: p.workDir})
		return nil, nil, fmt.Errorf("%w（StartUsn=%d < FirstUsn=%d），已重置游标", ErrCursorInvalid, startUsn, j.FirstUsn)
	}

	if startUsn >= j.NextUsn {
		return nil, nil, nil // 无待读变更
	}

	// 懒构建 FRN→路径缓存（仅首次有待读变更时；~8s 在启动后台 goroutine 可接受）
	if p.cache == nil {
		p.cache = newFrnPathCache(p.workDir)
		if err := p.cache.Build(ctx); err != nil {
			p.cache = nil
			return nil, nil, fmt.Errorf("FRN 缓存构建失败: %w", err)
		}
	}

	records, lastUsn, err := readJournalBatch(h, startUsn, j.NextUsn, j.UsnJournalID)
	if err != nil {
		return nil, nil, fmt.Errorf("读 USN journal 失败: %w", err)
	}

	changes := pairAndResolve(records, p.cache)

	// 存游标到已读进度（D6：早于 dispatch；崩溃在「存游标后 dispatch 前」漏报由对账兜底 D2）
	if err := p.cursors.Save(ctx, Cursor{JournalID: j.UsnJournalID, StartUsn: lastUsn, WorkDir: p.workDir}); err != nil {
		return nil, nil, fmt.Errorf("存 USN 游标失败: %w", err)
	}

	logger.Log.Infof("[fsmonitor] USN 续读完成：%d 条记录 → %d 条变更，游标 %d → %d",
		len(records), len(changes), startUsn, lastUsn)
	return changes, nil, nil
}

// volumeDevicePath 从 workDir 推导卷设备路径（E:\code → \\.\E:）。无盘符路径返回空（后续 openVolume 失败降级）。
func volumeDevicePath(workDir string) string {
	vol := filepath.VolumeName(workDir) // "E:"
	if len(vol) != 2 || vol[1] != ':' {
		return ""
	}
	return `\\.\` + vol
}

// openVolume 打开卷设备句柄（GENERIC_READ，需管理员；FILE_ATTRIBUTE_NORMAL，卷句柄不需 BACKUP_SEMANTICS）。
func openVolume(volume string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(volume)
	if err != nil {
		return 0, fmt.Errorf("卷路径转 UTF-16 失败: %w", err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, fsctlShareMode, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, fmt.Errorf("打开卷句柄失败（USN 卷级读取需管理员）: %w", err)
	}
	return h, nil
}

// queryJournal 查询 journal 元信息（UsnJournalID/FirstUsn/NextUsn）。METHOD_BUFFERED，栈结构体出参。
func queryJournal(h windows.Handle) (usnJournalData, error) {
	var out usnJournalData
	var returned uint32
	if err := windows.DeviceIoControl(h, fsctlQueryUSNJournal, nil, 0,
		(*byte)(unsafe.Pointer(&out)), uint32(unsafe.Sizeof(out)), &returned, nil); err != nil {
		return usnJournalData{}, fmt.Errorf("FSCTL_QUERY_USN_JOURNAL 失败: %w", err)
	}
	return out, nil
}

// readJournalBatch 从 startUsn 分批续读到 nextUsn，返回解析后的记录与已读到的最远 USN（lastUsn）。
// 输出缓冲堆分配 64KB；每批前 8 字节是下次续读起点，其后是 USN_RECORD 链（按 RecordLength 步进）。
// 终止：达 nextUsn / 达记录上限 / 出错 / 无进展（nextStart<=startUsn 防死循环）。
func readJournalBatch(h windows.Handle, startUsn, nextUsn int64, journalID uint64) (records []usnRecord, lastUsn int64, err error) {
	lastUsn = startUsn
	for len(records) < readJournalMaxRecords && startUsn < nextUsn {
		in := readUSNJournalDataV1{
			StartUsn:        startUsn,
			ReasonMask:      usnPathReasonMask,
			BytesToWaitFor:  0, // 0 = 到 journal 末尾即返回，不等待新记录
			UsnJournalID:    journalID,
			MinMajorVersion: 2, // 接受 V2
			MaxMajorVersion: 3, // 到 V3（V4/ReFS 留占位）
		}
		out := make([]byte, readJournalBatchSize) // 堆缓冲（METHOD_NEITHER）
		var returned uint32
		if e := windows.DeviceIoControl(h, fsctlReadUSNJournal,
			(*byte)(unsafe.Pointer(&in)), uint32(unsafe.Sizeof(in)),
			&out[0], uint32(len(out)), &returned, nil); e != nil {
			return records, lastUsn, e
		}
		if returned < 8 {
			break
		}
		nextStart := *(*int64)(unsafe.Pointer(&out[0])) // 前 8 字节 = 下次续读起点
		lastUsn = nextStart
		iterateRecords(out[8:returned], func(r usnRecord) { records = append(records, r) })
		if nextStart <= startUsn {
			break // 无进展防死循环
		}
		startUsn = nextStart
	}
	return records, lastUsn, nil
}

// renameLeg 待 NEW 配对的 rename 旧名腿。
type renameLeg struct {
	oldPath string    // 旧路径（Resolve 得）
	rec     usnRecord // 旧名记录（IsDir/FRN 供 OnDirRename/OnDirDelete 用）
}

// pairAndResolve 把一批 USN 记录（按提交序）转为 FileChange：
//   - rename OLD/NEW 按 FRN 在整批内配对（容忍乱序 R7），未配上兜底为 Remove(OLD)/Create(NEW)；
//   - 路径经 frnPathCache 解析，缓存随目录记录按序增量维护（OnDirCreate/OnDirDelete/OnDirRename）；
//   - 父目录不在缓存（workDir 子树外）的记录丢弃；
//   - 目录改名发 ChangeCreate(IsDir)（走 Correlator.processDirCreate，含空目录噪声抑制），不发 ChangeMove(IsDir)
//     （C-5 的 processMove 对 IsDir 返 nil）。
func pairAndResolve(records []usnRecord, cache *frnPathCache) []FileChange {
	var out []FileChange
	pendingOld := make(map[uint64]renameLeg) // FRN → 待配对旧名腿

	// emit 追加一条 FileChange，带白名单过滤（cache 解析出的路径本就在 store/* 子树，过滤作安全网）。
	emit := func(ch FileChange) {
		switch ch.Kind {
		case ChangeMove:
			if !storeRegistry.InScanDirs(ch.Path) && !storeRegistry.InScanDirs(ch.ToPath) {
				return
			}
		default:
			if !storeRegistry.InScanDirs(ch.Path) {
				return
			}
		}
		out = append(out, ch)
	}

	for _, r := range records {
		switch classifyRecord(r) {
		case usnClassIgnore:
			continue
		case usnClassRenameOld:
			oldPath, ok := cache.Resolve(r.ParentFRN, r.FileName)
			if !ok {
				continue // 父目录在 workDir 子树外，丢弃
			}
			pendingOld[r.FRN] = renameLeg{oldPath: oldPath, rec: r}
		case usnClassRenameNew:
			newPath, ok := cache.Resolve(r.ParentFRN, r.FileName)
			if !ok {
				// NEW 父目录在外部（移出 workDir，C-4.1 延后）：若有 OLD 配对则 OLD 按删除兜底
				if leg, hit := pendingOld[r.FRN]; hit {
					delete(pendingOld, r.FRN)
					if leg.rec.IsDir() {
						cache.OnDirDelete(leg.rec)
					} else {
						emit(FileChange{Kind: ChangeRemove, Path: leg.oldPath, IsDir: false, DetectedAt: ts(r)})
					}
				}
				continue
			}
			if leg, hit := pendingOld[r.FRN]; hit {
				delete(pendingOld, r.FRN)
				if r.IsDir() {
					cache.OnDirRename(leg.rec, r)
					// 目录改名走 processDirCreate（噪声抑制），发 ChangeCreate(IsDir)
					emit(FileChange{Kind: ChangeCreate, Path: newPath, IsDir: true, DetectedAt: ts(r)})
				} else {
					emit(FileChange{Kind: ChangeMove, Path: leg.oldPath, ToPath: newPath, IsDir: false, DetectedAt: ts(r)})
				}
			} else {
				// 无 OLD 配对：兜底为 Create
				if r.IsDir() {
					cache.OnDirCreate(r)
				}
				emit(FileChange{Kind: ChangeCreate, Path: newPath, IsDir: r.IsDir(), DetectedAt: ts(r)})
			}
		case usnClassCreate:
			path, ok := cache.Resolve(r.ParentFRN, r.FileName)
			if !ok {
				continue
			}
			if r.IsDir() {
				cache.OnDirCreate(r)
			}
			emit(FileChange{Kind: ChangeCreate, Path: path, IsDir: r.IsDir(), DetectedAt: ts(r)})
		case usnClassDelete:
			path, ok := cache.Resolve(r.ParentFRN, r.FileName)
			if !ok {
				continue
			}
			if r.IsDir() {
				cache.OnDirDelete(r)
				continue // 目录无 persistent_store 记录，不发 Change
			}
			emit(FileChange{Kind: ChangeRemove, Path: path, IsDir: false, DetectedAt: ts(r)})
		}
	}

	// 收尾：剩余 pendingOld（无 NEW 配对）兜底为 Remove
	for _, leg := range pendingOld {
		if leg.rec.IsDir() {
			cache.OnDirDelete(leg.rec)
		} else {
			emit(FileChange{Kind: ChangeRemove, Path: leg.oldPath, IsDir: false, DetectedAt: ts(leg.rec)})
		}
	}

	return out
}

// ts 把 USN 记录的 Windows FILETIME（自 1601 起，100ns 单位）转为 Unix 毫秒；0 用当前时间兜底。
func ts(r usnRecord) int64 {
	if r.TimeStamp == 0 {
		return now()
	}
	// 100ns → ms：/10000；Windows epoch(1601) → Unix(1970) 偏移 11644473600000 ms
	return int64(r.TimeStamp)/10000 - 11644473600000
}
