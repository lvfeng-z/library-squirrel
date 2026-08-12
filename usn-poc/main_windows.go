//go:build windows

// usn-poc（C-0a）：USN Journal 离线追溯可行性验证。
// 两条路径对照：
//   A. 卷句柄 + FSCTL_QUERY_USN_JOURNAL —— 离线追溯真正需要的路径（验证 R2 权限门槛）
//   B. 文件句柄 + FSCTL_READ_FILE_USN_DATA —— 对照实验（非管理员可开，验证 DeviceIoControl
//      机制正确 + USN_RECORD 解析正确 + USN 在本系统可用）
// B 若成功而 A 失败 → R2 失败定位为"卷句柄需 GENERIC_READ=管理员权限"，C 节点需重估。
// 临时 PoC，验证完可整体删除目录。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FSCTL 常量：x/sys/windows 未导出卷级 USN 查询码，按 winioctl.h 的 CTL_CODE 宏自算。
// CTL_CODE(DeviceType, Function, Method, Access) = (t<<16)|(a<<14)|(f<<2)|m
// 权威参数（MinGW/Wine winioctl.h 逐字镜像 Windows SDK）：
//   FSCTL_QUERY_USN_JOURNAL = CTL_CODE(FILE_DEVICE_FILE_SYSTEM, 61, METHOD_BUFFERED, FILE_ANY_ACCESS) → 0x000900F4
//   FSCTL_READ_FILE_USN_DATA = 0x000900EB（x/sys/windows 已导出，直接用）
const (
	fileDeviceFileSystem = 9
	methodBuffered       = 0
	fileAnyAccess        = 0
)

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

// usnJournalDataV0 FSCTL_QUERY_USN_JOURNAL 的输出结构（7 个 uint64，56 字节，自然对齐）。
type usnJournalDataV0 struct {
	UsnJournalID    uint64
	FirstUsn        int64
	NextUsn         int64
	LowestValidUsn  uint64
	MaxUsn          uint64
	MaximumSize     uint64
	AllocationDelta uint64
}

func main() {
	workDir := flag.String("workdir", defaultWorkDir(), "工作目录绝对路径（取其所在卷）")
	flag.Parse()

	volume := volumePath(*workDir)
	fmt.Printf("workDir = %s\n", *workDir)
	fmt.Printf("volume  = %s\n", volume)
	checkVolumeFS(volume)

	// A. 卷句柄 QUERY 路径（离线追溯所需）
	fmt.Println("\n=== A. 卷句柄 + FSCTL_QUERY_USN_JOURNAL（离线追溯路径）===")
	queryOK := queryJournalViaVolume(volume)

	// B. 文件句柄对照（验证机制 + USN 解析，非管理员）
	fmt.Println("\n=== B. 文件句柄 + FSCTL_READ_FILE_USN_DATA（对照实验）===")
	readFileUSN(filepath.Join(*workDir, "go.mod"))

	fmt.Println("\n=== 结论 ===")
	if queryOK {
		fmt.Println("R2 通过：非管理员可经卷句柄读 USN journal，可继续 C-0b。")
	} else {
		fmt.Println("R2 卷句柄路径受阻。对照 B 若成功 → 非管理员能经文件句柄读 USN 记录、解析正确，")
		fmt.Println("QUERY 失败定位为卷句柄需 GENERIC_READ（管理员）→ 离线追溯需管理员运行，C 节点需重估。")
	}
}

// queryJournalViaVolume 打开卷句柄并尝试 FSCTL_QUERY_USN_JOURNAL（多 function 候选），打印诊断。
func queryJournalViaVolume(volume string) bool {
	h, err := openVolumeAny(volume)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	for _, q := range []struct {
		name string
		val  uint32
	}{
		{"function61/0x000900F4", ctlCode(fileDeviceFileSystem, 61, methodBuffered, fileAnyAccess)},
		{"function44/0x000900B0", ctlCode(fileDeviceFileSystem, 44, methodBuffered, fileAnyAccess)},
	} {
		var out usnJournalDataV0
		var br uint32
		e := windows.DeviceIoControl(
			h, q.val, nil, 0,
			(*byte)(unsafe.Pointer(&out)), uint32(unsafe.Sizeof(out)),
			&br, nil,
		)
		if e == nil {
			fmt.Printf("[QUERY] %s 成功（返回 %d 字节）\n", q.name, br)
			fmt.Println("USN_JOURNAL_DATA:")
			fmt.Printf("  UsnJournalID = %d (0x%X)\n", out.UsnJournalID, out.UsnJournalID)
			fmt.Printf("  FirstUsn     = %d\n", out.FirstUsn)
			fmt.Printf("  NextUsn      = %d\n", out.NextUsn)
			fmt.Printf("  MaximumSize  = %d 字节（≈%.1f MB）\n", out.MaximumSize, float64(out.MaximumSize)/1024/1024)
			return true
		}
		fmt.Printf("[QUERY] %s → %v（errno=%d）\n", q.name, e, errnoOf(e))
	}
	return false
}

// readFileUSN 用 FSCTL_READ_FILE_USN_DATA 经文件句柄读单文件的 USN 记录（非管理员对照）。
func readFileUSN(filePath string) {
	utf16, _ := windows.UTF16PtrFromString(filePath)
	fh, err := windows.CreateFile(
		utf16, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)
	if err != nil {
		fmt.Printf("[对照] 打开文件失败 %s: %v（errno=%d）\n", filePath, err, errnoOf(err))
		return
	}
	defer windows.CloseHandle(fh)

	const fsctlReadFileUsnData = 0x000900EB
	buf := make([]byte, 65536) // 堆分配大缓冲，规避 METHOD_NEITHER 栈缓冲问题
	var br uint32
	err = windows.DeviceIoControl(fh, fsctlReadFileUsnData, nil, 0, &buf[0], uint32(len(buf)), &br, nil)
	if err != nil {
		fmt.Printf("[对照] FSCTL_READ_FILE_USN_DATA 失败: %v（errno=%d）\n", err, errnoOf(err))
		return
	}
	fmt.Printf("[对照] FSCTL_READ_FILE_USN_DATA 成功（%d 字节）\n", br)
	// FSCTL_READ_FILE_USN_DATA 输出直接是 USN_RECORD（无前缀；区别于卷级 READ_USN_JOURNAL 的"8字节USN + 记录"布局）
	if br < 60 {
		fmt.Printf("[对照] 返回过短: %d 字节\n", br)
		return
	}
	parseAndPrintUSNRecord(buf[:br])
}

// parseAndPrintUSNRecord 解析并打印 USN_RECORD（V2/V3）。
func parseAndPrintUSNRecord(rec []byte) {
	if len(rec) < 60 {
		fmt.Printf("[记录] 过短: %d 字节\n", len(rec))
		return
	}
	recLen := *(*uint32)(unsafe.Pointer(&rec[0]))
	major := *(*uint16)(unsafe.Pointer(&rec[4]))
	frn := *(*uint64)(unsafe.Pointer(&rec[8]))
	pfrn := *(*uint64)(unsafe.Pointer(&rec[16]))
	usn := *(*int64)(unsafe.Pointer(&rec[24]))
	reason := *(*uint32)(unsafe.Pointer(&rec[40]))
	nameLen := *(*uint16)(unsafe.Pointer(&rec[56])) // 字节
	nameOff := *(*uint16)(unsafe.Pointer(&rec[58])) // 字节，相对 record 起点
	var name string
	if int(nameOff)+int(nameLen) <= len(rec) && nameLen > 0 {
		n16 := unsafe.Slice((*uint16)(unsafe.Pointer(&rec[nameOff])), int(nameLen)/2)
		name = windows.UTF16ToString(n16)
	}
	fmt.Printf("[记录] V%d recLen=%d FRN=%d ParentFRN=%d USN=%d Reason=0x%X FileName=%q\n",
		major, recLen, frn, pfrn, usn, reason, name)
	fmt.Printf("[记录]   Reason 位: %s\n", reasonFlags(reason))
}

// reasonFlags 把 USN Reason 掩码解析为可读位名。
func reasonFlags(r uint32) string {
	flags := []struct {
		bit  uint32
		name string
	}{
		{0x00000100, "FILE_CREATE"},
		{0x00000200, "FILE_DELETE"},
		{0x00001000, "RENAME_OLD_NAME"},
		{0x00002000, "RENAME_NEW_NAME"},
		{0x00008000, "BASIC_INFO_CHANGE"},
		{0x80000000, "CLOSE"},
		{0x00000001, "DATA_OVERWRITE"},
		{0x00000002, "DATA_EXTEND"},
		{0x00000004, "DATA_TRUNCATION"},
	}
	var parts []string
	for _, f := range flags {
		if r&f.bit != 0 {
			parts = append(parts, f.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X(无已知位)", r)
	}
	return strings.Join(parts, "|")
}

// checkVolumeFS 打印卷文件系统类型与是否支持 USN journal（FILE_SUPPORTS_USN_JOURNAL=0x02000000）。
func checkVolumeFS(volume string) {
	root := strings.TrimPrefix(volume, `\\.\`) + `\`
	rootUTF16, _ := windows.UTF16PtrFromString(root)
	var fsFlags uint32
	var fsNameBuf [32]uint16
	err := windows.GetVolumeInformation(rootUTF16, nil, 0, nil, nil, &fsFlags, &fsNameBuf[0], uint32(len(fsNameBuf)))
	if err != nil {
		fmt.Printf("[卷信息] GetVolumeInformation 失败: %v\n", err)
		return
	}
	fsName := windows.UTF16ToString(fsNameBuf[:])
	supportsUSN := fsFlags&0x02000000 != 0
	fmt.Printf("[卷信息] %s 文件系统=%s, 支持 USN journal=%v\n", root, fsName, supportsUSN)
}

// openVolume 以指定访问掩码打开卷设备句柄（宽松共享，供 DeviceIoControl）。
func openVolume(volume string, access uint32) (windows.Handle, error) {
	volUTF16, _ := windows.UTF16PtrFromString(volume)
	return windows.CreateFile(
		volUTF16, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0,
	)
}

// openVolumeAny 依次试 GENERIC_READ 与 FILE_READ_ATTRIBUTES 打开卷句柄，返回首个成功者。
func openVolumeAny(volume string) (windows.Handle, error) {
	for _, mask := range []struct {
		name string
		val  uint32
	}{
		{"GENERIC_READ", windows.GENERIC_READ},
		{"FILE_READ_ATTRIBUTES", windows.FILE_READ_ATTRIBUTES},
	} {
		h, err := openVolume(volume, mask.val)
		if err == nil {
			fmt.Printf("[R2] %s 打开卷句柄成功\n", mask.name)
			return h, nil
		}
		fmt.Printf("[R2] %s 打开失败: %v（errno=%d）\n", mask.name, err, errnoOf(err))
	}
	return 0, fmt.Errorf("所有访问掩码均失败")
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

// defaultWorkDir 默认用程序运行目录（开发期=项目根）
func defaultWorkDir() string {
	wd, _ := os.Getwd()
	return wd
}
