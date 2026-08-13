//go:build windows

// usn-poc（C-0a + C-0b）：USN Journal 离线追溯可行性验证。临时 PoC，验证完可整体删除目录。
//
// C-0a（已完成）：FSCTL_QUERY_USN_JOURNAL 卷句柄权限验证（R2）+ 文件句柄对照（机制/解析正确）。
//   结论：卷级 USN 读取需管理员（R2 确认）；function 61/USN_RECORD 解析正确。
//
// C-0b（本文件主体）：在管理员环境下续读一批 USN 记录，量化两个未决风险以定 D4：
//   R1 FRN→全路径解析体量：USN_RECORD 只给 FileName + ParentFRN，须建 FRN→路径缓存才能拼相对 workDir 路径。
//      本 PoC 实测方案B（遍历 workDir 建缓存）的规模与耗时，并验证 ParentFRN 解析命中率。
//   R6 FRN 复用频率：NTFS 回收已删文件 FRN 给新建文件，复用会致缓存错映射。本 PoC 在记录流中检测
//      "同 FRN 先 DELETE 后 CREATE" 的复用模式，估频率以校准缓存窗口策略。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== FSCTL 常量（CTL_CODE 宏自算；x/sys/windows 未导出卷级 USN 查询码）=====
//
// CTL_CODE(DeviceType, Function, Method, Access) = (DeviceType<<16)|(Access<<14)|(Function<<2)|Method
// 权威参数（winioctl.h 逐字镜像 Windows SDK）：
//   FSCTL_QUERY_USN_JOURNAL  = CTL_CODE(FILE_DEVICE_FILE_SYSTEM, 61, METHOD_BUFFERED, FILE_ANY_ACCESS) = 0x000900F4
//   FSCTL_READ_USN_JOURNAL   = CTL_CODE(FILE_DEVICE_FILE_SYSTEM, 46, METHOD_NEITHER,  FILE_ANY_ACCESS) = 0x000900BB
//   FSCTL_READ_FILE_USN_DATA = CTL_CODE(FILE_DEVICE_FILE_SYSTEM, 58, METHOD_NEITHER,  FILE_ANY_ACCESS) = 0x000900EB
//
// METHOD_NEITHER：I/O 管理器不解缓冲，直接把用户态缓冲指针传给驱动——缓冲必须有效堆地址，
// 调用期间不可被回收（Go 的堆对象在 syscall 期间稳定，故用 make([]byte,...) 堆缓冲）。
const (
	fileDeviceFileSystem = 9
	methodBuffered       = 0
	methodNeither        = 3
	fileAnyAccess        = 0

	fileFlagBackupSemantics = 0x02000000 // 打开目录句柄必需（否则 CreateFile 拒绝目录）
)

var (
	fsctlQueryUSNJournal = ctlCode(fileDeviceFileSystem, 61, methodBuffered, fileAnyAccess)
	fsctlReadUSNJournal  = ctlCode(fileDeviceFileSystem, 46, methodNeither, fileAnyAccess)
	fsctlReadFileUSNData = ctlCode(fileDeviceFileSystem, 58, methodNeither, fileAnyAccess)
)

// USN Reason 位（winioctl.h；USN_RECORD.Reason 与 READ_USN_JOURNAL 的 ReasonMask 共用同一套位）。
const (
	reasonFileCreate = 0x00000100
	reasonFileDelete = 0x00000200
	reasonRenameOld  = 0x00001000
	reasonRenameNew  = 0x00002000
	reasonClose      = 0x80000000
)

// pathReasonMask 首版只追路径类变更（CREATE/DELETE/RENAME），与主方案决策3（内容修改延后）一致。
// 用于 READ_USN_JOURNAL 的 ReasonMask，让卷级续读只返回路径类记录，过滤掉海量数据修改噪声。
const pathReasonMask = reasonFileCreate | reasonFileDelete | reasonRenameOld | reasonRenameNew

// scanDirs 白名单（与 backend/fsmonitor/scanner.go:14-19 对齐）：
// USN 产出的路径须命中白名单才上报，store/ 外的变更（.git/log/backend 等）为噪声须丢弃。
// 建缓存也只覆盖白名单子树（实际成本口径），而非整个 workDir。
var scanDirs = []string{
	"store/resource",
	"store/thumbnail",
	"store/avatar/local",
	"store/avatar/site",
}

// fsctlShareMode 打开文件/卷句柄用的宽松共享掩码（允许其他进程读写删除，避免被占用打不开）。
const fsctlShareMode = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

func ctlCode(deviceType, function, method, access uint32) uint32 {
	return (deviceType << 16) | (access << 14) | (function << 2) | method
}

// errnoOf 从 windows 调用返回的 error 中提取 Win32 错误码。
func errnoOf(err error) uint32 {
	var errno windows.Errno
	if errors.As(err, &errno) {
		return uint32(errno)
	}
	return 0
}

// ===== 结构体 =====

// usnJournalDataV0 FSCTL_QUERY_USN_JOURNAL 输出（7 个 uint64，56 字节，自然对齐）。
type usnJournalDataV0 struct {
	UsnJournalID    uint64
	FirstUsn        int64
	NextUsn         int64
	LowestValidUsn  uint64
	MaxUsn          uint64
	MaximumSize     uint64
	AllocationDelta uint64
}

// readUSNJournalDataV1（48 字节，Win8+）FSCTL_READ_USN_JOURNAL 输入。
// 现代 NTFS 驱动要求 V1——旧 V0（__READ_USN_JOURNAL_DATA__，32 字节）会返回 ERROR_INVALID_USER_BUFFER(1784)：
// 驱动按 V1 布局读 InputBuffer，32 字节越界 + UsnJournalID 缺失即拒绝（C-0b 实测确认）。
// V1 相比 V0：Timeout/BytesToWaitFor 顺序互换；MinMaximumSize 替换为 UsnJournalID + Min/MaxMajorVersion
// （支持 USN_RECORD V3/V4 版本范围选择）。UsnJournalID 须与 FSCTL_QUERY_USN_JOURNAL 返回值一致，否则失败。
// 布局：@0 StartUsn(i64) @8 ReasonMask(u32) @12 ReturnOnlyOnClose(u32) @16 Timeout(u64) @24 BytesToWaitFor(u64)
//       @32 UsnJournalID(u64) @40 MinMajorVersion(u16) @42 MaxMajorVersion(u16)（44 + 4 padding → 48，8 字节对齐）
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

// usnRecord 从 USN_RECORD（V2/V3，字段偏移相同）解析出的结构化记录。
// USN_RECORD 布局：0=RecordLength(u32) 4=Major(u16) 6=Minor(u16) 8=FRN(u64)
//   16=ParentFRN(u64) 24=USN(i64) 32=TimeStamp(u64) 40=Reason(u32) 44=SourceInfo(u32)
//   48=SecurityId(u32) 52=FileAttributes(u32) 56=FileNameLength(u16,字节) 58=FileNameOffset(u16,字节) 60=FileName(UTF16)
type usnRecord struct {
	RecLen    uint32
	Major     uint16
	FRN       uint64
	ParentFRN uint64
	USN       int64
	Reason    uint32
	FileName  string
}

func main() {
	workDir := flag.String("workdir", defaultWorkDir(), "工作目录绝对路径（取其所在卷）")
	maxRecords := flag.Int("maxrecords", 5000, "PoC 采样记录上限（防跑太久）")
	flag.Parse()

	absWorkDir, _ := filepath.Abs(*workDir)
	volume := volumePath(absWorkDir)
	fmt.Printf("workDir = %s\n", absWorkDir)
	fmt.Printf("volume  = %s\n", volume)
	checkVolumeFS(volume)

	// === A. QUERY（C-0a 复用）：拿 journal 元信息，FirstUsn 作 READ 起点 ===
	jd, ok := queryJournal(volume)
	if !ok {
		fmt.Println("\n[致命] QUERY 卷句柄失败（R2：卷级 USN 需管理员权限）。C-0b 无法继续。")
		fmt.Println("       请以管理员身份运行 usn-poc/run_admin.bat。")
		os.Exit(1)
	}
	fmt.Printf("\n[QUERY] UsnJournalID=%d FirstUsn=%d NextUsn=%d（journal 跨度 %.1f MB）\n",
		jd.UsnJournalID, jd.FirstUsn, jd.NextUsn, float64(jd.NextUsn-jd.FirstUsn)/1024/1024)

	// 记录活动前 journal 末尾，作活动段 READ 起点
	preActUsn := jd.NextUsn

	// === B. 触发 workDir 文件活动（生成已知 USN 记录，供 R1 解析确定性验证）===
	fmt.Println("\n=== B. 触发 workDir 文件活动（create/rename）===")
	triggerWorkDirActivity(absWorkDir)
	time.Sleep(200 * time.Millisecond) // 等 USN 记录落盘到 journal，避免 actEndUsn 截断触发记录
	jd2, ok2 := queryJournal(volume)
	var actEndUsn int64
	if ok2 {
		actEndUsn = jd2.NextUsn
		fmt.Printf("  活动期间 USN 区间 [%d, %d]（新增 %d 字节记录）\n", preActUsn, actEndUsn, actEndUsn-preActUsn)
	}

	// === C. [R1] 建 FRN→路径缓存（活动后建，含触发的 .usn-poc-trigger 目录）===
	fmt.Println("\n=== C. [R1] 建 FRN→路径缓存（方案B，全 workDir）===")
	cache, buildStat := buildFRNCache(absWorkDir)
	fmt.Printf("  全 workDir：%d 文件 / %d 目录 → %d 缓存条目，耗时 %v\n",
		buildStat.fullFileCount, buildStat.fullDirCount, len(cache), buildStat.elapsed)
	fmt.Printf("  白名单 store/* 子树：%d 文件 / %d 目录（生产上报口径；开发环境常为空）\n",
		buildStat.fileCount, buildStat.dirCount)

	// === D. [READ-活动段] 读活动期间记录，验证 R1 解析（应确定性命中）===
	fmt.Println("\n=== D. [READ-活动段] R1 解析验证 ===")
	actRecords, actStat := readJournalBatch(volume, preActUsn, actEndUsn, jd.UsnJournalID, 5000)
	fmt.Printf("  活动段 %d 批 IO，%d 条路径类记录\n", actStat.batches, len(actRecords))
	printReasonDist(actRecords)
	actResolve := resolveAndSample(actRecords, cache)
	fmt.Printf("  解析：全缓存命中 %d / 未命中 %d（命中率 %.1f%%），白名单内 %d\n",
		actResolve.hit, actResolve.miss, actResolve.hitRate(), actResolve.wlHit)

	// === E. [READ-历史段] 读历史大样本，R6 复用检测 ===
	fmt.Println("\n=== E. [READ-历史段] R6 复用检测（大样本）===")
	histRecords, histStat := readJournalBatch(volume, jd.FirstUsn, preActUsn, jd.UsnJournalID, *maxRecords)
	fmt.Printf("  历史段 %d 批 IO，%d 条路径类记录（读到 NextUsn=%d）\n", histStat.batches, len(histRecords), histStat.lastNextUsn)
	reuseStat := detectFRNReuse(histRecords)
	fmt.Printf("  唯一 FRN = %d，出现多次的 FRN = %d\n", reuseStat.uniqueFRN, reuseStat.repeatedFRN)
	fmt.Printf("  检测到 delete→create 复用 = %d 例\n", reuseStat.deleteThenCreate)

	// === 结论（D4 判据） ===
	printConclusion(buildStat, actResolve, reuseStat, len(histRecords))

	// 清理 PoC 触发的临时目录
	_ = os.RemoveAll(filepath.Join(absWorkDir, ".usn-poc-trigger"))
}

// ===== A. QUERY =====

// queryJournal 打开卷句柄（需管理员 GENERIC_READ）并 FSCTL_QUERY_USN_JOURNAL，返回 journal 元信息。
func queryJournal(volume string) (usnJournalDataV0, bool) {
	h, err := openVolume(volume, windows.GENERIC_READ)
	if err != nil {
		fmt.Printf("[QUERY] 打开卷句柄失败: %v（errno=%d，疑似非管理员，R2）\n", err, errnoOf(err))
		return usnJournalDataV0{}, false
	}
	defer windows.CloseHandle(h)

	var out usnJournalDataV0
	var br uint32
	if err := windows.DeviceIoControl(
		h, fsctlQueryUSNJournal, nil, 0,
		(*byte)(unsafe.Pointer(&out)), uint32(unsafe.Sizeof(out)),
		&br, nil,
	); err != nil {
		fmt.Printf("[QUERY] FSCTL_QUERY_USN_JOURNAL 失败: %v（errno=%d）\n", err, errnoOf(err))
		return usnJournalDataV0{}, false
	}
	return out, true
}

// ===== B. R1 建缓存 =====

type buildStats struct {
	fullFileCount int // 全 workDir 文件数（建全缓存口径；白名单体量 ≤ 此上界）
	fullDirCount  int
	fileCount     int // 白名单命中文件数（生产上报口径）
	dirCount      int // 白名单命中目录数
	elapsed       time.Duration
}

// buildFRNCache 遍历 workDir，对全树条目经 FSCTL_READ_FILE_USN_DATA 读 FRN 建 FRN→路径 缓存，
// 并单独计白名单命中数。PoC 建全缓存（而非只白名单）以验证解析机制——开发环境白名单 store/* 常为空，
// 全缓存可证"ParentFRN→路径解析在真实记录上可行"，且全 workDir 体量是白名单子集的上界。
// 缓存含目录条目是关键——USN_RECORD.ParentFRN 指向父目录，须有目录 FRN 才能拼路径。
func buildFRNCache(workDir string) (map[uint64]string, buildStats) {
	cache := make(map[uint64]string)
	var st buildStats
	start := time.Now()
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(workDir, path)
		relSlash := filepath.ToSlash(rel)
		isDir := info.IsDir()
		if isDir {
			st.fullDirCount++
		} else {
			st.fullFileCount++
		}
		// 读 FRN（READ_FILE_USN_DATA 非管理员可读；目录需 FILE_FLAG_BACKUP_SEMANTICS）
		frn, err := readFRN(path)
		if err != nil {
			return nil
		}
		cache[frn] = relSlash
		if inScanDirs(relSlash) {
			if isDir {
				st.dirCount++
			} else {
				st.fileCount++
			}
		}
		return nil
	})
	st.elapsed = time.Since(start)
	return cache, st
}

// triggerWorkDirActivity 在 workDir 内创建临时目录并做 create/rename，生成已知 USN 记录供 R1 解析
// 确定性验证。保留目录与一个文件，使其 FRN 进入活动后建的缓存（解析时 ParentFRN 可命中）。
func triggerWorkDirActivity(workDir string) {
	base := filepath.Join(workDir, ".usn-poc-trigger")
	_ = os.RemoveAll(base) // 清理上次 PoC 残留（幂等）
	if err := os.MkdirAll(base, 0o755); err != nil {
		fmt.Printf("  [触发] 创建目录失败: %v\n", err)
		return
	}
	f1 := filepath.Join(base, "tmp1.txt")
	f2 := filepath.Join(base, "tmp2.txt")
	_ = os.WriteFile(f1, []byte("usn-poc-trigger"), 0o644)       // CREATE tmp1.txt
	_ = os.Rename(f1, f2)                                         // RENAME_OLD tmp1 → RENAME_NEW tmp2
	_ = os.WriteFile(f2, []byte("usn-poc-trigger-keep"), 0o644)  // 占位保留 tmp2.txt（DATA 类记录不在路径 ReasonMask，仅留文件入缓存）
	fmt.Printf("  [触发] 已在 %s 做 create/rename（保留目录+tmp2.txt 进缓存）\n", base)
	// 打印 base 目录 FRN（诊断锚点：活动后建缓存读同值入 map，tmp 记录的 ParentFRN 应等于此）
	if frn, err := readFRN(base); err == nil {
		fmt.Printf("  [触发] base 目录 FRN=%d（诊断锚点：tmp 记录 ParentFRN 应等于此）\n", frn)
	} else {
		fmt.Printf("  [触发] base 目录 readFRN 失败: %v\n", err)
	}
}

// readFRN 经文件/目录句柄发 FSCTL_READ_FILE_USN_DATA，取 USN_RECORD 偏移 8 的 FRN（uint64）。
// 输出布局：直接是 USN_RECORD（无前缀；区别于卷级 READ_USN_JOURNAL 的"8字节USN + 记录"布局）。
func readFRN(path string) (uint64, error) {
	utf16, _ := windows.UTF16PtrFromString(path)
	// 打开目录须 FILE_FLAG_BACKUP_SEMANTICS，否则 CreateFile 返回 ERROR_ACCESS_DENIED
	flags := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if isDirPath(path) {
		flags |= fileFlagBackupSemantics
	}
	fh, err := windows.CreateFile(utf16, windows.GENERIC_READ, fsctlShareMode, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(fh)

	buf := make([]byte, 65536) // 堆缓冲，规避 METHOD_NEITHER 栈缓冲问题
	var br uint32
	if err := windows.DeviceIoControl(fh, fsctlReadFileUSNData, nil, 0, &buf[0], uint32(len(buf)), &br, nil); err != nil {
		return 0, err
	}
	if br < 16 { // 至少需读到 FRN 字段（偏移 8，8 字节）
		return 0, fmt.Errorf("READ_FILE_USN_DATA 返回过短 %d 字节", br)
	}
	return *(*uint64)(unsafe.Pointer(&buf[8])), nil
}

func isDirPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// inScanDirs 判断 workDir 相对路径是否命中任一白名单子树（含自身）。
func inScanDirs(rel string) bool {
	if rel == "." {
		return false
	}
	for _, d := range scanDirs {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// ===== C. READ 续读 =====

type readStats struct {
	batches     int
	lastNextUsn int64
}

// readJournalBatch 从 firstUsn 续读到 nextUsn，分批 FSCTL_READ_USN_JOURNAL，累计采样最多 maxRecords 条。
// 每批输出布局：前 8 字节 = 下次续读起点（next StartUsn），其后是 USN_RECORD 链（按 RecordLength 步进）。
func readJournalBatch(volume string, firstUsn, nextUsn int64, journalID uint64, maxRecords int) ([]usnRecord, readStats) {
	var st readStats
	h, err := openVolume(volume, windows.GENERIC_READ)
	if err != nil {
		fmt.Printf("[READ] 打开卷句柄失败: %v（errno=%d）\n", err, errnoOf(err))
		return nil, st
	}
	defer windows.CloseHandle(h)

	var records []usnRecord
	startUsn := firstUsn
	for len(records) < maxRecords && startUsn < nextUsn {
		in := readUSNJournalDataV1{
			StartUsn:        startUsn,
			ReasonMask:      pathReasonMask,
			BytesToWaitFor:  0, // 0 = 到达 journal 末尾即成功返回，不等待新记录
			UsnJournalID:    journalID,
			MinMajorVersion: 2, // 接受 USN_RECORD V2
			MaxMajorVersion: 3, // 到 V3（V4/ReFS 128-bit FRN 留占位，首版不处理）
		}
		out := make([]byte, 65536)
		var br uint32
		err := windows.DeviceIoControl(
			h, fsctlReadUSNJournal,
			(*byte)(unsafe.Pointer(&in)), uint32(unsafe.Sizeof(in)),
			&out[0], uint32(len(out)),
			&br, nil,
		)
		if err != nil {
			fmt.Printf("[READ] 第 %d 批失败: %v（errno=%d），停止续读\n", st.batches+1, err, errnoOf(err))
			break
		}
		st.batches++
		if br < 8 {
			break
		}
		nextStart := *(*int64)(unsafe.Pointer(&out[0]))
		st.lastNextUsn = nextStart
		// 解析 out[8:br] 的 USN_RECORD 链
		off := 8
		for off+60 <= int(br) {
			recLen := *(*uint32)(unsafe.Pointer(&out[off]))
			if recLen == 0 || off+int(recLen) > int(br) {
				break
			}
			records = append(records, parseRecord(out[off:off+int(recLen)]))
			off += int(recLen) // USN_RECORD 已 QWORD(8字节)对齐，RecordLength 含填充，直接累加
		}
		if nextStart <= startUsn {
			break // 无进展防死循环
		}
		startUsn = nextStart
	}
	return records, st
}

// parseRecord 从 USN_RECORD 原始字节按版本（V2/V3）解析出结构化记录。
// V3 与 V2 的关键差异：FileReferenceNumber/ParentFileReferenceNumber 各占 128 位（FILE_ID_128，
// 为 ReFS 128-bit 文件 ID 预留；NTFS 上高 64 位为 0），其后所有字段偏移 +16 字节。
// 按 V2 偏移解析 V3 记录：ParentFRN@16 读到 FRN 高半(=0)、Reason/FileName 全错位（C-0b 实测确认）。
func parseRecord(rec []byte) usnRecord {
	var r usnRecord
	if len(rec) < 60 {
		return r
	}
	r.RecLen = *(*uint32)(unsafe.Pointer(&rec[0]))
	r.Major = *(*uint16)(unsafe.Pointer(&rec[4]))
	r.FRN = *(*uint64)(unsafe.Pointer(&rec[8])) // V2/V3 均为 FRN 低 64 位（NTFS 足够；readFRN 同口径）
	switch r.Major {
	case 3:
		// V3：@8 FRN(128) @24 ParentFRN(128) @40 Usn @48 TimeStamp @56 Reason
		//     @60 SourceInfo @64 SecurityId @68 FileAttributes @72 FileNameLength @74 FileNameOffset @76 FileName
		if len(rec) < 76 {
			return r
		}
		r.ParentFRN = *(*uint64)(unsafe.Pointer(&rec[24]))
		r.USN = *(*int64)(unsafe.Pointer(&rec[40]))
		r.Reason = *(*uint32)(unsafe.Pointer(&rec[56]))
		r.FileName = decodeName(rec, int(*(*uint16)(unsafe.Pointer(&rec[74]))), int(*(*uint16)(unsafe.Pointer(&rec[72]))))
	default:
		// V2（及未知版本兜底）：@8 FRN(64) @16 ParentFRN(64) @24 Usn @32 TimeStamp @40 Reason
		//     @44 SourceInfo @48 SecurityId @52 FileAttributes @56 FileNameLength @58 FileNameOffset @60 FileName
		r.ParentFRN = *(*uint64)(unsafe.Pointer(&rec[16]))
		r.USN = *(*int64)(unsafe.Pointer(&rec[24]))
		r.Reason = *(*uint32)(unsafe.Pointer(&rec[40]))
		r.FileName = decodeName(rec, int(*(*uint16)(unsafe.Pointer(&rec[58]))), int(*(*uint16)(unsafe.Pointer(&rec[56]))))
	}
	return r
}

// decodeName 从 record 的 off 偏移读取 length 字节的 UTF-16 文件名（length 为字节数）。
func decodeName(rec []byte, off, length int) string {
	if length <= 0 || off <= 0 || off+length > len(rec) {
		return ""
	}
	n16 := unsafe.Slice((*uint16)(unsafe.Pointer(&rec[off])), length/2)
	return windows.UTF16ToString(n16)
}

// printReasonDist 打印记录的 Reason 位分布统计。
func printReasonDist(records []usnRecord) {
	if len(records) == 0 {
		fmt.Println("  （无记录）")
		return
	}
	dist := map[string]int{}
	for _, r := range records {
		for _, b := range []struct {
			bit  uint32
			name string
		}{
			{reasonFileCreate, "CREATE"},
			{reasonFileDelete, "DELETE"},
			{reasonRenameOld, "RENAME_OLD"},
			{reasonRenameNew, "RENAME_NEW"},
			{reasonClose, "CLOSE"},
		} {
			if r.Reason&b.bit != 0 {
				dist[b.name]++
			}
		}
	}
	parts := make([]string, 0, len(dist))
	for k := range dist {
		parts = append(parts, fmt.Sprintf("%s=%d", k, dist[k]))
	}
	sort.Strings(parts)
	fmt.Printf("  Reason 分布：%s\n", strings.Join(parts, " "))
}

// ===== D. R1 解析验证 =====

type resolveStats struct {
	hit   int // ParentFRN 命中全 workDir 缓存（解析机制可行：能拼出 workDir 相对路径）
	wlHit int // 命中且父目录在白名单内（真实上报量；store/* 外的变更正确丢弃）
	miss  int // ParentFRN 未命中（变更发生在 workDir 之外，正确丢弃）
}

func (s resolveStats) hitRate() float64 {
	tot := s.hit + s.miss
	if tot == 0 {
		return 0
	}
	return float64(s.hit) * 100 / float64(tot)
}

// resolveAndSample 用全 workDir 缓存解析每条记录的 ParentFRN，统计命中/未命中与白名单占比，
// 并打印前若干命中样本（人眼校验解析正确性）。
func resolveAndSample(records []usnRecord, cache map[uint64]string) resolveStats {
	var st resolveStats
	const sampleMax = 15
	sampled := 0
	for _, r := range records {
		parentPath, ok := cache[r.ParentFRN]
		if !ok {
			st.miss++
			// 未命中也打印前若干条原始字段，对比 base FRN 诊断为何 ParentFRN 对不上缓存键
			if sampled < sampleMax {
				fmt.Printf("  [未命中] V%d FRN=%d ParentFRN=%d Reason=%s Name=%q\n",
					r.Major, r.FRN, r.ParentFRN, reasonName(r.Reason), r.FileName)
				sampled++
			}
			continue
		}
		st.hit++
		inWL := inScanDirs(parentPath)
		if inWL {
			st.wlHit++
		}
		if sampled < sampleMax {
			full := parentPath + "/" + r.FileName
			tag := "命中"
			if inWL {
				tag = "命中·白名单"
			}
			fmt.Printf("  [%s] %s  (V%d FRN=%d ← ParentFRN=%d, Reason=%s)\n", tag, full, r.Major, r.FRN, r.ParentFRN, reasonName(r.Reason))
			sampled++
		}
	}
	return st
}

// reasonName 给 Reason 掩码一个简短可读名（首版路径类）。
func reasonName(r uint32) string {
	var parts []string
	for _, b := range []struct {
		bit  uint32
		name string
	}{
		{reasonFileCreate, "CREATE"},
		{reasonFileDelete, "DELETE"},
		{reasonRenameOld, "RENAME_OLD"},
		{reasonRenameNew, "RENAME_NEW"},
		{reasonClose, "CLOSE"},
	} {
		if r&b.bit != 0 {
			parts = append(parts, b.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X", r)
	}
	return strings.Join(parts, "|")
}

// ===== E. R6 复用检测 =====

type reuseStats struct {
	uniqueFRN        int // 出现过的不同 FRN 数
	repeatedFRN      int // 出现 ≥2 次的 FRN 数
	deleteThenCreate int // 同 FRN 序列中先 DELETE 后 CREATE 的复用例数
}

// detectFRNReuse 按 FRN 聚合记录（按 USN 升序），检测"先 DELETE 后 CREATE"——FRN 被回收复用的特征。
func detectFRNReuse(records []usnRecord) reuseStats {
	type sighting struct {
		usn    int64
		reason uint32
	}
	byFRN := map[uint64][]sighting{}
	for _, r := range records {
		byFRN[r.FRN] = append(byFRN[r.FRN], sighting{r.USN, r.Reason})
	}
	var st reuseStats
	st.uniqueFRN = len(byFRN)
	for _, ss := range byFRN {
		if len(ss) > 1 {
			st.repeatedFRN++
		}
		sort.Slice(ss, func(i, j int) bool { return ss[i].usn < ss[j].usn })
		sawDelete := false
		for _, s := range ss {
			if s.reason&reasonFileDelete != 0 {
				sawDelete = true
			}
			if s.reason&reasonFileCreate != 0 && sawDelete {
				st.deleteThenCreate++
				break
			}
		}
	}
	return st
}

// ===== 结论 =====

// printConclusion 综合 R1/R6 实测，给出 D4（FRN 路径解析是否需 derive 独立基建节点）的判据建议。
func printConclusion(buildStat buildStats, resolveStat resolveStats, reuseStat reuseStats, totalRecords int) {
	fmt.Println("\n=== 结论（D4 判据）===")
	fmt.Printf("R1 建缓存：全 workDir %d 文件/%d 目录 → %d 缓存条目，耗时 %v\n",
		buildStat.fullFileCount, buildStat.fullDirCount, buildStat.fullFileCount+buildStat.fullDirCount, buildStat.elapsed)
	fmt.Printf("         （白名单 store/* 子树：%d 文件/%d 目录——生产实际上报口径；开发环境常为空）\n",
		buildStat.fileCount, buildStat.dirCount)
	fmt.Printf("R1 解析  ：全缓存命中率 %.1f%%（命中=%d/未命中=%d），白名单内 %d 条\n",
		resolveStat.hitRate(), resolveStat.hit, resolveStat.miss, resolveStat.wlHit)
	fmt.Printf("R6 复用  ：唯一 FRN %d，复用例 %d（观察样本 %d 条）\n",
		reuseStat.uniqueFRN, reuseStat.deleteThenCreate, totalRecords)

	fmt.Println("\nD4 建议：")
	if resolveStat.hit > 0 {
		fmt.Println("  解析机制可行：ParentFRN 命中全缓存即拼出 workDir 相对路径（样本已人眼校验）。")
	}
	if buildStat.fullFileCount > 100000 {
		fmt.Printf("  全 workDir %d 文件建缓存耗时 %v；生产实际只缓存白名单 store/* 子树（≤ 全 workDir），\n", buildStat.fullFileCount, buildStat.elapsed)
		fmt.Println("  真实大库（store/resource 海量文件）耗时须用户侧实测。")
		fmt.Println("  → 倾向：FRN 解析无需独立基建节点，C-2 直接实现；若用户侧实测大库耗时不可接受，再 derive。")
	} else {
		fmt.Printf("  全 workDir %d 文件建缓存耗时 %v，体量可接受（白名单子树 ≤ 此）。\n", buildStat.fullFileCount, buildStat.elapsed)
		fmt.Println("  → FRN 解析无需独立基建节点，C-2 直接实现。")
	}
	if reuseStat.uniqueFRN > 0 && reuseStat.deleteThenCreate > 0 {
		pct := float64(reuseStat.deleteThenCreate) * 100 / float64(reuseStat.uniqueFRN)
		fmt.Printf("  R6 实测：%d 例 delete→create 复用（占唯一 FRN %.1f%%）——复用高频非低频，\n", reuseStat.deleteThenCreate, pct)
		fmt.Println("        缓存必须落实「删除记录到达即清条目」防御（方案 §4.2），不可省略。")
	} else {
		fmt.Println("  R6 样本未捕获复用；缓存仍须落实「删除记录到达即清条目」防御（方案 §4.2），不可省略。")
	}
	fmt.Println("  注2：D1 游标表/D2 对账兜底/D3 ChangeMove 分支 不受本 PoC 影响，可并行推进。")
}

// ===== 卷/路径辅助 =====

// checkVolumeFS 打印卷文件系统类型与是否支持 USN journal（FILE_SUPPORTS_USN_JOURNAL=0x02000000）。
func checkVolumeFS(volume string) {
	root := strings.TrimPrefix(volume, `\\.\`) + `\`
	rootUTF16, _ := windows.UTF16PtrFromString(root)
	var fsFlags uint32
	var fsNameBuf [32]uint16
	if err := windows.GetVolumeInformation(rootUTF16, nil, 0, nil, nil, &fsFlags, &fsNameBuf[0], uint32(len(fsNameBuf))); err != nil {
		fmt.Printf("[卷信息] GetVolumeInformation 失败: %v\n", err)
		return
	}
	fmt.Printf("[卷信息] %s 文件系统=%s, 支持 USN journal=%v\n",
		root, windows.UTF16ToString(fsNameBuf[:]), fsFlags&0x02000000 != 0)
}

// openVolume 以指定访问掩码打开卷设备句柄（宽松共享，供 DeviceIoControl）。
func openVolume(volume string, access uint32) (windows.Handle, error) {
	volUTF16, _ := windows.UTF16PtrFromString(volume)
	return windows.CreateFile(
		volUTF16, access, fsctlShareMode,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)
}

// volumePath 从路径推卷设备路径：E:\code → \\.\E:
func volumePath(p string) string {
	abs, _ := filepath.Abs(p)
	if len(abs) < 2 || abs[1] != ':' {
		fmt.Printf("无法从路径推断盘符: %s\n", p)
		os.Exit(2)
	}
	return `\\.\` + string(abs[0]) + `:`
}

// defaultWorkDir 默认用程序运行目录（开发期=项目根）。
func defaultWorkDir() string {
	wd, _ := os.Getwd()
	return wd
}
