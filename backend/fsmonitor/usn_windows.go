//go:build windows

// USN Journal 离线追溯的记录解析层（节点 C 的 C-1：纯解析，不涉及卷句柄）。
// 本文件只做「原始字节 → 结构化记录 + 变更语义分类」，卷句柄打开、FSCTL 续读、
// 游标持久化、FRN→路径解析、rename 配对编排均在后续阶段（C-2/C-3/C-4）实现。
//
// USN_RECORD 有 V2/V3（及 ReFS 的 V4）三版本，须按 Major 版本分支解析：
// V3 的 FileReferenceNumber/ParentFileReferenceNumber 各占 128 位（FILE_ID_128，
// 为 ReFS 128-bit 文件 ID 预留；NTFS 上高 64 位恒 0），使 V3 记录较 V2 整体后移 +16B。
// 按 V2 偏移解析 V3 会令 ParentFRN 读成 FRN 高半(=0)、Reason/FileName 全错位。
package fsmonitor

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// USN Reason 位（winioctl.h；USN_RECORD.Reason 与 READ_USN_JOURNAL 的 ReasonMask 共用同一套位）。
// 首版只追路径类变更，数据修改类（USN_REASON_DATA_*）不在此列。
const (
	usnReasonFileCreate = 0x00000100 // USN_REASON_FILE_CREATE
	usnReasonFileDelete = 0x00000200 // USN_REASON_FILE_DELETE
	usnReasonRenameOld  = 0x00001000 // USN_REASON_FILE_RENAME_OLD_NAME
	usnReasonRenameNew  = 0x00002000 // USN_REASON_FILE_RENAME_NEW_NAME
)

// fileAttributeDirectory FILE_ATTRIBUTE_DIRECTORY（winnt.h），FileAttributes 中该位置位表示目录。
const fileAttributeDirectory = 0x00000010

// usnRecord 从 USN_RECORD 原始字节（V2/V3）解析出的结构化记录。
// FRN/ParentFRN 取低 64 位（NTFS 上 FILE_ID_128 高 64 位恒 0，低 64 位即文件引用号）。
type usnRecord struct {
	RecLen         uint32 // RecordLength（含 QWORD 对齐填充）
	Major          uint16 // USN_RECORD 主版本（2 或 3）
	FRN            uint64 // 文件引用号（低 64 位）
	ParentFRN      uint64 // 父目录引用号（低 64 位），拼相对 workDir 全路径的线索
	USN            int64  // 更新序号，单调递增，作续读游标
	TimeStamp      uint64 // 记录时间（Windows 100ns 间隔，自 1601 起），供乱序/关联参考
	Reason         uint32 // 变更原因位掩码
	FileAttributes uint32 // 文件属性
	FileName       string // 单级文件名（UTF-16 解码后），不含路径分隔符
}

// IsDir 记录是否为目录（据 FileAttributes 的 FILE_ATTRIBUTE_DIRECTORY 位）。
func (r usnRecord) IsDir() bool { return r.FileAttributes&fileAttributeDirectory != 0 }

// parseRecord 从一条 USN_RECORD 原始字节按 Major 版本解析。
// 返回 (record, true) 成功；(零值, false) 记录过短、长度不自洽或版本不支持（V4/未知）。
func parseRecord(rec []byte) (usnRecord, bool) {
	// 共用前缀：@0 RecordLength(u32) @4 Major(u16) @8 FRN(u64 低64，V2/V3 同口径)
	if len(rec) < 16 {
		return usnRecord{}, false
	}
	r := usnRecord{
		RecLen: *(*uint32)(unsafe.Pointer(&rec[0])),
		Major:  *(*uint16)(unsafe.Pointer(&rec[4])),
		FRN:    *(*uint64)(unsafe.Pointer(&rec[8])),
	}
	// RecordLength 须覆盖整条记录且不越界（步进与切片边界一致性的前置校验）
	if int(r.RecLen) > len(rec) || r.RecLen < 8 {
		return usnRecord{}, false
	}
	switch r.Major {
	case 3:
		// V3 布局：@8 FRN(128) @24 ParentFRN(128) @40 USN @48 TimeStamp
		//         @56 Reason @68 FileAttributes @72 FileNameLength @74 FileNameOffset @76 FileName
		if len(rec) < 76 {
			return usnRecord{}, false
		}
		r.ParentFRN = *(*uint64)(unsafe.Pointer(&rec[24]))
		r.USN = *(*int64)(unsafe.Pointer(&rec[40]))
		r.TimeStamp = *(*uint64)(unsafe.Pointer(&rec[48]))
		r.Reason = *(*uint32)(unsafe.Pointer(&rec[56]))
		r.FileAttributes = *(*uint32)(unsafe.Pointer(&rec[68]))
		nameLen := *(*uint16)(unsafe.Pointer(&rec[72]))
		nameOff := *(*uint16)(unsafe.Pointer(&rec[74]))
		r.FileName = decodeName(rec, int(nameOff), int(nameLen))
	case 2:
		// V2 布局：@8 FRN(64) @16 ParentFRN(64) @24 USN @32 TimeStamp
		//         @40 Reason @52 FileAttributes @56 FileNameLength @58 FileNameOffset @60 FileName
		if len(rec) < 60 {
			return usnRecord{}, false
		}
		r.ParentFRN = *(*uint64)(unsafe.Pointer(&rec[16]))
		r.USN = *(*int64)(unsafe.Pointer(&rec[24]))
		r.TimeStamp = *(*uint64)(unsafe.Pointer(&rec[32]))
		r.Reason = *(*uint32)(unsafe.Pointer(&rec[40]))
		r.FileAttributes = *(*uint32)(unsafe.Pointer(&rec[52]))
		nameLen := *(*uint16)(unsafe.Pointer(&rec[56]))
		nameOff := *(*uint16)(unsafe.Pointer(&rec[58]))
		r.FileName = decodeName(rec, int(nameOff), int(nameLen))
	default:
		// V4（ReFS）/未知版本：首版不支持，跳过交上层记录，不按 V2 兜底误解析
		return usnRecord{}, false
	}
	return r, true
}

// decodeName 从记录的 off 偏移读取 length 字节的 UTF-16 文件名（length 为字节数）。
// 边界不合法（零长度、越界）返回空串。
func decodeName(rec []byte, off, length int) string {
	if length <= 0 || off <= 0 || off+length > len(rec) {
		return ""
	}
	n16 := unsafe.Slice((*uint16)(unsafe.Pointer(&rec[off])), length/2)
	return windows.UTF16ToString(n16)
}

// iterateRecords 遍历 USN_RECORD 链，对每条成功解析的记录调用 fn。
// buf 为 FSCTL_READ_USN_JOURNAL 输出去掉前 8 字节「下次续读起点」后的纯记录链字节；
// 记录按 RecordLength（含 QWORD 对齐填充）步进。
func iterateRecords(buf []byte, fn func(usnRecord)) {
	off := 0
	for {
		if off+4 > len(buf) {
			return
		}
		recLen := int(*(*uint32)(unsafe.Pointer(&buf[off])))
		if recLen <= 0 || off+recLen > len(buf) {
			return
		}
		if rec, ok := parseRecord(buf[off : off+recLen]); ok {
			fn(rec)
		}
		off += recLen
	}
}

// usnChangeClass 单条 USN 记录的路径类变更分类。
// rename 的旧名/新名是两条独立记录，须在 provider 层按 FRN 配对合并为 ChangeMove；
// 此处仅标记腿别，不直接产出 ChangeMove。
type usnChangeClass int

const (
	usnClassIgnore    usnChangeClass = iota // 非路径类（数据修改等），丢弃
	usnClassCreate                          // 新建
	usnClassDelete                          // 删除
	usnClassRenameOld                       // rename 旧名腿，待 provider 配对
	usnClassRenameNew                       // rename 新名腿，待 provider 配对
)

// classifyRecord 将记录 Reason 掩码分类为路径类变更语义。
// 优先级 renameNew > renameOld > create > delete：单记录多位置位时 rename 优先于增删，
// 保证 rename 腿被识别为 rename 而非误判为增删（rename-new 记录通常不含 create 位，
// 此优先级仅作多位置位时的稳健兜底）。
func classifyRecord(r usnRecord) usnChangeClass {
	switch {
	case r.Reason&usnReasonRenameNew != 0:
		return usnClassRenameNew
	case r.Reason&usnReasonRenameOld != 0:
		return usnClassRenameOld
	case r.Reason&usnReasonFileCreate != 0:
		return usnClassCreate
	case r.Reason&usnReasonFileDelete != 0:
		return usnClassDelete
	default:
		return usnClassIgnore
	}
}
