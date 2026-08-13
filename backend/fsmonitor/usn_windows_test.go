//go:build windows

package fsmonitor

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// encodeName 将字符串编码为 UTF-16LE 字节序列（镜像 USN_RECORD.FileName 存储）。
func encodeName(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	b := make([]byte, len(u16)*2)
	for i, c := range u16 {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

// buildUSNRecord 构造一条 USN_RECORD 原始字节（按 major 选 V2/V3 布局，含 QWORD 对齐填充），
// 供 parseRecord 解析验证。字段偏移与 usn_windows.go parseRecord 一致。
func buildUSNRecord(major uint16, frn, parentFRN uint64, usn int64, ts uint64, reason, attrs uint32, name string) []byte {
	nameBytes := encodeName(name)
	var nameLenOff, nameOffOff, fileStart int
	switch major {
	case 3:
		nameLenOff, nameOffOff, fileStart = 72, 74, 76
	default: // V2
		nameLenOff, nameOffOff, fileStart = 56, 58, 60
	}
	total := fileStart + len(nameBytes)
	if pad := total % 8; pad != 0 {
		total += 8 - pad
	}
	rec := make([]byte, total)
	binary.LittleEndian.PutUint32(rec[0:], uint32(total))
	binary.LittleEndian.PutUint16(rec[4:], major)
	binary.LittleEndian.PutUint16(rec[6:], 0) // Minor
	binary.LittleEndian.PutUint64(rec[8:], frn)
	if major == 3 {
		binary.LittleEndian.PutUint64(rec[16:], 0) // FRN 高 64 位（NTFS 恒 0）
		binary.LittleEndian.PutUint64(rec[24:], parentFRN)
		binary.LittleEndian.PutUint64(rec[32:], 0) // ParentFRN 高 64 位
		binary.LittleEndian.PutUint64(rec[40:], uint64(usn))
		binary.LittleEndian.PutUint64(rec[48:], ts)
		binary.LittleEndian.PutUint32(rec[56:], reason)
		binary.LittleEndian.PutUint32(rec[60:], 0) // SourceInfo
		binary.LittleEndian.PutUint32(rec[64:], 0) // SecurityId
		binary.LittleEndian.PutUint32(rec[68:], attrs)
	} else {
		binary.LittleEndian.PutUint64(rec[16:], parentFRN)
		binary.LittleEndian.PutUint64(rec[24:], uint64(usn))
		binary.LittleEndian.PutUint64(rec[32:], ts)
		binary.LittleEndian.PutUint32(rec[40:], reason)
		binary.LittleEndian.PutUint32(rec[44:], 0) // SourceInfo
		binary.LittleEndian.PutUint32(rec[48:], 0) // SecurityId
		binary.LittleEndian.PutUint32(rec[52:], attrs)
	}
	binary.LittleEndian.PutUint16(rec[nameLenOff:], uint16(len(nameBytes)))
	binary.LittleEndian.PutUint16(rec[nameOffOff:], uint16(fileStart))
	copy(rec[fileStart:], nameBytes)
	return rec
}

// TestParseRecord_V2 验证 V2 记录各字段解析正确。
func TestParseRecord_V2(t *testing.T) {
	rec := buildUSNRecord(2, 0x1111, 0x2222, 700, 0x55AA, usnReasonFileCreate, 0, "a.txt")
	r, ok := parseRecord(rec)
	if !ok {
		t.Fatal("期望解析成功")
	}
	if r.Major != 2 || r.FRN != 0x1111 || r.ParentFRN != 0x2222 {
		t.Fatalf("版本/FRN/ParentFRN 错误: %+v", r)
	}
	if r.USN != 700 {
		t.Fatalf("USN 错误: got %d", r.USN)
	}
	if r.TimeStamp != 0x55AA {
		t.Fatalf("TimeStamp 错误: got 0x%X", r.TimeStamp)
	}
	if r.Reason != usnReasonFileCreate {
		t.Fatalf("Reason 错误: got 0x%X", r.Reason)
	}
	if r.FileName != "a.txt" {
		t.Fatalf("FileName 错误: got %q", r.FileName)
	}
	if int(r.RecLen) != len(rec) {
		t.Fatalf("RecLen 错误: got %d want %d", r.RecLen, len(rec))
	}
}

// TestParseRecord_V3 验证 V3 记录各字段解析正确（重点：ParentFRN 读自 @24 非 @16）。
func TestParseRecord_V3(t *testing.T) {
	rec := buildUSNRecord(3, 0x1111, 0x2222, 900, 0x55AA, usnReasonFileDelete, fileAttributeDirectory, "sub")
	r, ok := parseRecord(rec)
	if !ok {
		t.Fatal("期望解析成功")
	}
	if r.Major != 3 || r.FRN != 0x1111 || r.ParentFRN != 0x2222 {
		t.Fatalf("版本/FRN/ParentFRN 错误: %+v", r)
	}
	if r.USN != 900 {
		t.Fatalf("USN 错误: got %d", r.USN)
	}
	if r.TimeStamp != 0x55AA {
		t.Fatalf("TimeStamp 错误: got 0x%X", r.TimeStamp)
	}
	if r.Reason != usnReasonFileDelete {
		t.Fatalf("Reason 错误: got 0x%X", r.Reason)
	}
	if r.FileName != "sub" {
		t.Fatalf("FileName 错误: got %q", r.FileName)
	}
	if !r.IsDir() {
		t.Fatal("期望 IsDir=true")
	}
}

// TestParseRecord_V3LayoutTrap 直接守住 V3 布局陷阱（R4）：
// V3 @16 是 FRN 高半(=0)、@24 才是 ParentFRN；@40 是 USN、@56 才是 Reason。
// 若误用 V2 偏移，ParentFRN 会读成 0、Reason 会读成 USN 低半。本测试用刻意区分的值断言无误读。
func TestParseRecord_V3LayoutTrap(t *testing.T) {
	// FRN/ParentFRN/USN/Reason 取互不重叠的可辨识值，任一字段错位都会暴露
	rec := buildUSNRecord(3, 0xAAAA, 0xBBBB, 0x1234, 0xC0FFEE, usnReasonRenameOld, 0, "trap.dat")
	r, ok := parseRecord(rec)
	if !ok {
		t.Fatal("期望解析成功")
	}
	if r.ParentFRN != 0xBBBB {
		t.Fatalf("V3 ParentFRN 须读自 @24(=0xBBBB)；误用 V2 偏移会读 @16 的 FRN 高半(=0)，got 0x%X", r.ParentFRN)
	}
	if r.Reason != usnReasonRenameOld {
		t.Fatalf("V3 Reason 须读自 @56；误用 V2 偏移会读 @40 的 USN 低半，got 0x%X", r.Reason)
	}
	if r.USN != 0x1234 {
		t.Fatalf("USN 错误: got 0x%X", r.USN)
	}
}

// TestParseRecord_UTF16Name 验证 V2/V3 的 UTF-16 文件名（含非 ASCII 中文）解码正确。
func TestParseRecord_UTF16Name(t *testing.T) {
	name := "作品_001.jpg"
	for _, major := range []uint16{2, 3} {
		rec := buildUSNRecord(major, 1, 2, 1, 0, usnReasonFileCreate, 0, name)
		r, ok := parseRecord(rec)
		if !ok {
			t.Fatalf("V%d 期望解析成功", major)
		}
		if r.FileName != name {
			t.Fatalf("V%d FileName 解码错误: got %q want %q", major, r.FileName, name)
		}
	}
}

// TestParseRecord_Invalid 验证过短记录、长度不自洽、未知版本均解析失败。
func TestParseRecord_Invalid(t *testing.T) {
	cases := []struct {
		name string
		rec  []byte
	}{
		{"空", nil},
		{"不足16字节共用前缀", make([]byte, 10)},
		{"V2不足60", buildUSNRecord(2, 1, 2, 3, 0, 0, 0, "x")[:30]},
		{"V3不足76", buildUSNRecord(3, 1, 2, 3, 0, 0, 0, "x")[:50]},
	}
	for _, c := range cases {
		if _, ok := parseRecord(c.rec); ok {
			t.Fatalf("%s：期望解析失败却成功", c.name)
		}
	}
	// 未知版本（V4）：构造最小合法前缀但 Major=4
	unk := make([]byte, 64)
	binary.LittleEndian.PutUint32(unk[0:], 64)
	binary.LittleEndian.PutUint16(unk[4:], 4)
	binary.LittleEndian.PutUint64(unk[8:], 1)
	if _, ok := parseRecord(unk); ok {
		t.Fatal("V4/未知版本：期望解析失败（首版不支持）")
	}
}

// TestDecodeName_EdgeCases 验证文件名解码的边界保护。
func TestDecodeName_EdgeCases(t *testing.T) {
	rec := buildUSNRecord(2, 1, 2, 3, 0, 0, 0, "ok.txt")
	if decodeName(rec, 0, 4) != "" {
		t.Fatal("off=0 应返回空（USN 偏移从正数起）")
	}
	if decodeName(rec, 60, 0) != "" {
		t.Fatal("length=0 应返回空")
	}
	if decodeName(rec, 60, 9999) != "" {
		t.Fatal("越界 length 应返回空")
	}
	// "ok.txt" 6 字符 = 12 字节 UTF-16LE；decodeName 的 length 为字节数
	if decodeName(rec, 60, 12) != "ok.txt" {
		t.Fatalf("合法输入应解码正确: got %q", decodeName(rec, 60, 12))
	}
}

// TestIterateRecords 验证记录链遍历：V2 与 V3 混排、按 RecordLength 步进、全部命中回调。
func TestIterateRecords(t *testing.T) {
	r1 := buildUSNRecord(2, 0x10, 0x20, 100, 0, usnReasonFileCreate, 0, "v2.txt")
	r2 := buildUSNRecord(3, 0x30, 0x40, 200, 0, usnReasonRenameNew, fileAttributeDirectory, "v3dir")
	r3 := buildUSNRecord(2, 0x50, 0x60, 300, 0, usnReasonFileDelete, 0, "gone.txt")
	// 拼成纯记录链（不含 READ_USN_JOURNAL 8 字节前缀，前缀处理归 C-4）
	buf := append(append(r1, r2...), r3...)
	var got []usnRecord
	iterateRecords(buf, func(r usnRecord) { got = append(got, r) })
	if len(got) != 3 {
		t.Fatalf("期望遍历 3 条，got %d", len(got))
	}
	if got[0].FRN != 0x10 || got[0].FileName != "v2.txt" {
		t.Fatalf("第1条错误: %+v", got[0])
	}
	if got[1].FRN != 0x30 || got[1].Major != 3 || !got[1].IsDir() {
		t.Fatalf("第2条错误: %+v", got[1])
	}
	if got[2].FRN != 0x50 || got[2].FileName != "gone.txt" {
		t.Fatalf("第3条错误: %+v", got[2])
	}
}

// TestClassifyRecord 验证 Reason 掩码到变更语义的分类（含多位置位与噪声位）。
func TestClassifyRecord(t *testing.T) {
	cases := []struct {
		name   string
		reason uint32
		want   usnChangeClass
	}{
		{"新建", usnReasonFileCreate, usnClassCreate},
		{"删除", usnReasonFileDelete, usnClassDelete},
		{"rename旧名", usnReasonRenameOld, usnClassRenameOld},
		{"rename新名", usnReasonRenameNew, usnClassRenameNew},
		// rename-new 记录常伴随 CLOSE(0x80000000)，应忽略噪声位仍判 renameNew
		{"rename新名+CLOSE", usnReasonRenameNew | 0x80000000, usnClassRenameNew},
		// 多位置位：rename 优先于 create
		{"rename新名+create", usnReasonRenameNew | usnReasonFileCreate, usnClassRenameNew},
		{"rename旧名+delete", usnReasonRenameOld | usnReasonFileDelete, usnClassRenameOld},
		// 纯噪声（数据修改类，不在路径 ReasonMask 内）→ 忽略
		{"仅CLOSE", 0x80000000, usnClassIgnore},
		{"仅数据修改", 0x00000001, usnClassIgnore},
	}
	for _, c := range cases {
		r := usnRecord{Reason: c.reason}
		if got := classifyRecord(r); got != c.want {
			t.Fatalf("%s：分类错误 got %v want %v (reason=0x%X)", c.name, got, c.want, c.reason)
		}
	}
}

// TestIsDir 验证据 FileAttributes 判定目录。
func TestIsDir(t *testing.T) {
	if !(usnRecord{FileAttributes: fileAttributeDirectory}.IsDir()) {
		t.Fatal("目录属性位应判 IsDir=true")
	}
	if (usnRecord{FileAttributes: 0}.IsDir()) {
		t.Fatal("无目录属性位应判 IsDir=false")
	}
	// 其他属性位（如只读 0x01）不影响目录判定
	if (usnRecord{FileAttributes: 0x00000011}.IsDir()) == false {
		t.Fatal("含目录位+其他位应判 IsDir=true")
	}
}

// TestInScanDirs 验证白名单谓词：命中 store/* 子树为 true，外部目录为 false。
func TestInScanDirs(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"store/resource/作者/x.jpg", true},
		{"store/resource", true}, // 子树根自身
		{"store/thumbnail/t.jpg", true},
		{"store/avatar/local/a.png", true},
		{"store/avatar/site/b.png", true},
		{"store/avatar", false}, // 仅 store/avatar 不在白名单（只有 local/site）
		{"backup/2026/x.mp4", false},
		{".git/config", false},
		{"log/server.log", false},
		{"", false},
		{".", false},
		{"store/resourceX/y.jpg", false}, // 前缀串匹配须按分隔符，非 store/resourceX
	}
	for _, c := range cases {
		if got := inScanDirs(c.rel); got != c.want {
			t.Fatalf("inScanDirs(%q) = %v want %v", c.rel, got, c.want)
		}
	}
}
