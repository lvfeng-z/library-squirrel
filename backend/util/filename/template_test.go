package filename

import (
	"database/sql"
	"testing"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
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
		DownloadYear:    "2026",
		DownloadMonth:   "05",
		DownloadDay:     "18",
		DownloadHour:    "12",
		DownloadMinute:  "00",
		DownloadSecond:  "00",
	}

	tpl := "[${author}]_[${siteWorkId}]_${siteWorkName}_${description}_${localAuthorName}_${siteAuthorName}_${siteAuthorId}_${uploadTimeYear}${uploadTimeMonth}${uploadTimeDay}_${downloadTimeYear}${downloadTimeMonth}${downloadTimeDay}"
	expected := "[TestAuthor]_[work456]_MyWork_A test work_LocalAuthor_SiteAuthor_author123_20260518_20260518"
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

// --- ExtractTokenData 测试 ---

func strPtr(s string) *string { return &s }

func TestExtractTokenData_NilResponse(t *testing.T) {
	data := ExtractTokenData(nil)
	if data.Author != fallbackAuthor {
		t.Errorf("Author = %q, want %q", data.Author, fallbackAuthor)
	}
	if data.SiteWorkID != "" {
		t.Errorf("SiteWorkID = %q, want empty", data.SiteWorkID)
	}
}

func TestExtractTokenData_SiteAuthorOnly(t *testing.T) {
	resp := &dto.WorkResponse{
		Work: &entity2.Work{
			BaseEntity:       &model.BaseEntity{},
			SiteWorkID:       sql.NullString{String: "art123", Valid: true},
			SiteWorkName:     sql.NullString{String: "Test Art", Valid: true},
			SiteUploadTime:   sql.NullInt64{Int64: 1779542400000, Valid: true}, // 2026-05-21 00:00:00 UTC
			SiteAuthorID:     sql.NullString{String: "author456", Valid: true},
			SiteWorkDescription: sql.NullString{String: "desc", Valid: true},
		},
		SiteAuthors: []*dto.TaskSiteAuthorDTO{
			{SiteAuthorID: "1", AuthorName: "PixivArtist"},
		},
	}

	data := ExtractTokenData(resp)
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
	if data.UploadYear != "2026" {
		t.Errorf("UploadYear = %q, want %q", data.UploadYear, "2026")
	}
}

func TestExtractTokenData_LocalAuthorPreferred(t *testing.T) {
	resp := &dto.WorkResponse{
		Work: &entity2.Work{BaseEntity: &model.BaseEntity{}},
		LocalAuthors: []*dto.LocalAuthorDTO{
			{AuthorName: strPtr("LocalArtist")},
		},
		SiteAuthors: []*dto.TaskSiteAuthorDTO{
			{AuthorName: "SiteArtist"},
		},
	}

	data := ExtractTokenData(resp)
	if data.Author != "LocalArtist" {
		t.Errorf("Author = %q, want %q (local author should be preferred)", data.Author, "LocalArtist")
	}
}

func TestExtractTokenData_NoAuthors(t *testing.T) {
	resp := &dto.WorkResponse{
		Work: &entity2.Work{BaseEntity: &model.BaseEntity{}},
	}

	data := ExtractTokenData(resp)
	if data.Author != fallbackAuthor {
		t.Errorf("Author = %q, want %q", data.Author, fallbackAuthor)
	}
}

func TestExtractTokenData_EmptyAuthorName(t *testing.T) {
	resp := &dto.WorkResponse{
		Work: &entity2.Work{BaseEntity: &model.BaseEntity{}},
		LocalAuthors: []*dto.LocalAuthorDTO{
			{AuthorName: strPtr("")},
		},
		SiteAuthors: []*dto.TaskSiteAuthorDTO{
			{AuthorName: ""},
		},
	}

	data := ExtractTokenData(resp)
	if data.Author != fallbackAuthor {
		t.Errorf("Author = %q, want %q", data.Author, fallbackAuthor)
	}
}

func TestExtractTokenData_DownloadTime(t *testing.T) {
	data := ExtractTokenData(&dto.WorkResponse{
		Work: &entity2.Work{BaseEntity: &model.BaseEntity{}},
	})
	if data.DownloadYear == "" {
		t.Error("DownloadYear should not be empty")
	}
	if len(data.DownloadYear) != 4 {
		t.Errorf("DownloadYear = %q, want 4-digit year", data.DownloadYear)
	}
}

// --- 集成测试：完整流程 ---

func TestFullFlow_TemplateWithSanitize(t *testing.T) {
	resp := &dto.WorkResponse{
		Work: &entity2.Work{
			BaseEntity:   &model.BaseEntity{},
			SiteWorkID:   sql.NullString{String: "12345", Valid: true},
			SiteWorkName: sql.NullString{String: "Test: Art*Work?", Valid: true},
		},
		SiteAuthors: []*dto.TaskSiteAuthorDTO{
			{AuthorName: "Artist<Name>"},
		},
	}

	data := ExtractTokenData(resp)
	tpl := "[${author}]_[${siteWorkId}]_${siteWorkName}"
	result := FormatFileName(tpl, data)
	sanitized := SanitizeFileName(result)

	expected := "[Artist＜Name＞]_[12345]_Test： Art＊Work？"
	if sanitized != expected {
		t.Errorf("Full flow result = %q, want %q", sanitized, expected)
	}
}
