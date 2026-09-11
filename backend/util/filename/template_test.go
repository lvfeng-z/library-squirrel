package filename

import (
	"testing"
	"time"
)

// --- SanitizeFileName 测试 ---

func TestSanitizeFileName_IllegalChars(t *testing.T) {
	input := `test<file>name:with*illegal?chars"/\|end`
	expected := "test＜file＞name：with＊illegal？chars＂／＼｜end"
	result := SanitizeFileName(input)
	if result != expected {
		t.Errorf("SanitizeFileName(%q) = %q, want %q", input, result, expected)
	}
}

func TestSanitizeFileName_LegalChars(t *testing.T) {
	input := "normal_file-name.txt"
	result := SanitizeFileName(input)
	if result != input {
		t.Errorf("SanitizeFileName(%q) = %q, want unchanged", input, result)
	}
}

func TestSanitizeFileName_EmptyString(t *testing.T) {
	result := SanitizeFileName("")
	if result != "" {
		t.Errorf("SanitizeFileName(\"\") = %q, want \"\"", result)
	}
}

func TestSanitizeFileName_EachChar(t *testing.T) {
	cases := map[string]string{
		`\`: "＼",
		`/`: "／",
		`:`: "：",
		`*`: "＊",
		`?`: "？",
		`"`: "＂",
		`<`: "＜",
		`>`: "＞",
		`|`: "｜",
	}
	for input, expected := range cases {
		result := SanitizeFileName(input)
		if result != expected {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", input, result, expected)
		}
	}
}

// --- FormatFileName 测试 ---

func TestFormatFileName_AllTokens(t *testing.T) {
	data := &TokenData{
		Author:          "TestAuthor",
		LocalAuthorName: "LocalAuthor",
		SiteAuthorName:  "SiteAuthor",
		SiteAuthorID:    "author123",
		SiteWorkID:      "work456",
		SiteWorkName:    "MyWork",
		Description:     "A test work",
		UploadYear:      "2026",
		UploadMonth:     "05",
		UploadDay:       "18",
		UploadHour:      "10",
		UploadMinute:    "30",
		UploadSecond:    "45",
		ExportYear:      "2026",
		ExportMonth:     "09",
		ExportDay:       "11",
		ExportHour:      "12",
		ExportMinute:    "00",
		ExportSecond:    "00",
	}

	tpl := "[${author}]_[${siteWorkId}]_${siteWorkName}_${description}_${localAuthorName}_${siteAuthorName}_${siteAuthorId}_${uploadTimeYear}${uploadTimeMonth}${uploadTimeDay}_${exportTimeYear}${exportTimeMonth}${exportTimeDay}"
	expected := "[TestAuthor]_[work456]_MyWork_A test work_LocalAuthor_SiteAuthor_author123_20260518_20260911"
	result := FormatFileName(tpl, data)
	if result != expected {
		t.Errorf("FormatFileName() = %q, want %q", result, expected)
	}
}

func TestFormatFileName_UnknownToken(t *testing.T) {
	data := &TokenData{Author: "A"}
	result := FormatFileName("${author}_${unknownToken}", data)
	if result != "A_${unknownToken}" {
		t.Errorf("FormatFileName() = %q, want %q", result, "A_${unknownToken}")
	}
}

// TestFormatFileName_DownloadTimeNotRecognized 旧占位符 downloadTime* 已改名为 exportTime*：
// 旧写法按未识别占位符原样保留，杜绝新旧两套占位符并存
func TestFormatFileName_DownloadTimeNotRecognized(t *testing.T) {
	data := &TokenData{Author: "A", ExportYear: "2026"}
	result := FormatFileName("${downloadTimeYear}_${exportTimeYear}", data)
	if result != "${downloadTimeYear}_2026" {
		t.Errorf("FormatFileName() = %q, want %q", result, "${downloadTimeYear}_2026")
	}
}

func TestFormatFileName_EmptyTemplate(t *testing.T) {
	result := FormatFileName("", &TokenData{Author: "A"})
	if result != "" {
		t.Errorf("FormatFileName(\"\", ...) = %q, want \"\"", result)
	}
}

func TestFormatFileName_NoTokens(t *testing.T) {
	result := FormatFileName("plain_name", &TokenData{})
	if result != "plain_name" {
		t.Errorf("FormatFileName() = %q, want %q", result, "plain_name")
	}
}

func TestFormatFileName_NilData(t *testing.T) {
	result := FormatFileName("${author}", nil)
	if result != "${author}" {
		t.Errorf("FormatFileName(..., nil) = %q, want %q", result, "${author}")
	}
}

// --- ExtractTokenData 测试（中立入参） ---

func TestExtractTokenData_EmptyFields(t *testing.T) {
	data := ExtractTokenData(WorkFields{})
	if data.Author != fallbackAuthor {
		t.Errorf("Author = %q, want %q", data.Author, fallbackAuthor)
	}
	if data.LocalAuthorName != fallbackAuthor || data.SiteAuthorName != fallbackAuthor {
		t.Errorf("作者名应回退占位值，实际 local=%q site=%q", data.LocalAuthorName, data.SiteAuthorName)
	}
	if data.SiteWorkID != "" || data.SiteWorkName != "" || data.Description != "" || data.SiteAuthorID != "" {
		t.Error("作品字段零值应为空串")
	}
}

func TestExtractTokenData_SiteAuthorOnly(t *testing.T) {
	data := ExtractTokenData(WorkFields{
		SiteWorkID:      "art123",
		SiteWorkName:    "Test Art",
		SiteAuthorID:    "author456",
		Description:     "desc",
		SiteAuthorNames: []string{"PixivArtist"},
	})
	if data.Author != "PixivArtist" {
		t.Errorf("Author = %q, want %q", data.Author, "PixivArtist")
	}
	if data.SiteAuthorName != "PixivArtist" {
		t.Errorf("SiteAuthorName = %q, want %q", data.SiteAuthorName, "PixivArtist")
	}
	if data.LocalAuthorName != fallbackAuthor {
		t.Errorf("LocalAuthorName = %q, want %q", data.LocalAuthorName, fallbackAuthor)
	}
	if data.SiteWorkID != "art123" {
		t.Errorf("SiteWorkID = %q, want %q", data.SiteWorkID, "art123")
	}
	if data.SiteWorkName != "Test Art" {
		t.Errorf("SiteWorkName = %q, want %q", data.SiteWorkName, "Test Art")
	}
	if data.Description != "desc" {
		t.Errorf("Description = %q, want %q", data.Description, "desc")
	}
}

func TestExtractTokenData_LocalAuthorPreferred(t *testing.T) {
	data := ExtractTokenData(WorkFields{
		LocalAuthorNames: []string{"LocalArtist"},
		SiteAuthorNames:  []string{"SiteArtist"},
	})
	if data.Author != "LocalArtist" {
		t.Errorf("Author = %q, want %q (本地作者优先)", data.Author, "LocalArtist")
	}
}

func TestExtractTokenData_EmptyAuthorNames(t *testing.T) {
	data := ExtractTokenData(WorkFields{
		LocalAuthorNames: []string{""},
		SiteAuthorNames:  []string{""},
	})
	if data.Author != fallbackAuthor {
		t.Errorf("Author = %q, want %q", data.Author, fallbackAuthor)
	}
}

// TestExtractTokenData_UploadTime 上传时间组件按本地时区格式化；0=未提供组件留空
func TestExtractTokenData_UploadTime(t *testing.T) {
	ms := int64(1779542400000)
	data := ExtractTokenData(WorkFields{UploadTimeMs: ms})
	want := time.UnixMilli(ms)
	if data.UploadYear != want.Format("2006") || data.UploadMonth != want.Format("01") || data.UploadDay != want.Format("02") {
		t.Errorf("UploadDate = %s-%s-%s, want %s", data.UploadYear, data.UploadMonth, data.UploadDay, want.Format("2006-01-02"))
	}
	if data.UploadHour != want.Format("15") || data.UploadMinute != want.Format("04") || data.UploadSecond != want.Format("05") {
		t.Errorf("UploadTime 组件应按本地时区格式化，实际 %s:%s:%s", data.UploadHour, data.UploadMinute, data.UploadSecond)
	}

	empty := ExtractTokenData(WorkFields{})
	if empty.UploadYear != "" || empty.UploadSecond != "" {
		t.Error("未提供上传时间（0）时组件应留空")
	}
}

// TestExtractTokenData_ExportTime 导出时间组件取入参基准时刻（非取当前时间，
// 供消费方锚定同输入同输出）
func TestExtractTokenData_ExportTime(t *testing.T) {
	ms := int64(1725000000000)
	data := ExtractTokenData(WorkFields{ExportTimeMs: ms})
	want := time.UnixMilli(ms)
	if data.ExportYear != want.Format("2006") || data.ExportMonth != want.Format("01") || data.ExportDay != want.Format("02") {
		t.Errorf("ExportDate = %s-%s-%s, want %s", data.ExportYear, data.ExportMonth, data.ExportDay, want.Format("2006-01-02"))
	}
	if data.ExportHour != want.Format("15") || data.ExportMinute != want.Format("04") || data.ExportSecond != want.Format("05") {
		t.Errorf("ExportTime 组件应与基准时刻一致，实际 %s:%s:%s", data.ExportHour, data.ExportMinute, data.ExportSecond)
	}
}

// --- 集成测试：完整流程 ---

func TestFullFlow_TemplateWithSanitize(t *testing.T) {
	data := ExtractTokenData(WorkFields{
		SiteWorkID:      "12345",
		SiteWorkName:    "Test: Art*Work?",
		SiteAuthorNames: []string{"Artist<Name>"},
	})
	tpl := "[${author}]_[${siteWorkId}]_${siteWorkName}"
	result := FormatFileName(tpl, data)
	sanitized := SanitizeFileName(result)

	expected := "[Artist＜Name＞]_[12345]_Test： Art＊Work？"
	if sanitized != expected {
		t.Errorf("Full flow result = %q, want %q", sanitized, expected)
	}
}
