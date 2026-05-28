package filename

import (
	"fmt"
	"strings"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
)

// TokenData 模板占位符对应的数据
type TokenData struct {
	Author          string
	LocalAuthorName string
	SiteAuthorName  string
	SiteAuthorID    string
	SiteWorkID      string
	SiteWorkName    string
	Description     string
	UploadYear      string
	UploadMonth     string
	UploadDay       string
	UploadHour      string
	UploadMinute    string
	UploadSecond    string
	DownloadYear    string
	DownloadMonth   string
	DownloadDay     string
	DownloadHour    string
	DownloadMinute  string
	DownloadSecond  string
}

const fallbackAuthor = "unknownAuthor"

// ExtractTokenData 从 WorkResponse 提取所有模板占位符的值
func ExtractTokenData(workResp *sdkdto.WorkResponse) *TokenData {
	data := &TokenData{}

	if workResp == nil {
		setDefaults(data)
		fillDownloadTime(data)
		return data
	}

	// 作者名称
	data.LocalAuthorName = extractLocalAuthorName(workResp.LocalAuthors)
	data.SiteAuthorName = extractSiteAuthorName(workResp.SiteAuthors)
	data.SiteAuthorID = ptrStringValue(workResp.Work, func(w *sdkdto.WorkDTO) string {
		return ptrStr(w.SiteAuthorID)
	})

	// ${author}: 优先本地作者，其次站点作者
	if data.LocalAuthorName != fallbackAuthor {
		data.Author = data.LocalAuthorName
	} else {
		data.Author = data.SiteAuthorName
	}

	// 作品字段
	data.SiteWorkID = ptrStringValue(workResp.Work, func(w *sdkdto.WorkDTO) string {
		return ptrStr(w.SiteWorkID)
	})
	data.SiteWorkName = ptrStringValue(workResp.Work, func(w *sdkdto.WorkDTO) string {
		return ptrStr(w.SiteWorkName)
	})
	data.Description = ptrStringValue(workResp.Work, func(w *sdkdto.WorkDTO) string {
		return ptrStr(w.SiteWorkDescription)
	})

	// 时间
	fillUploadTime(data, workResp.Work)
	fillDownloadTime(data)

	return data
}

// FormatFileName 将模板中的 ${...} 占位符替换为实际值，未识别的占位符保持原样
func FormatFileName(tpl string, data *TokenData) string {
	if tpl == "" || data == nil {
		return tpl
	}

	r := strings.NewReplacer(
		"${author}", data.Author,
		"${localAuthorName}", data.LocalAuthorName,
		"${siteAuthorName}", data.SiteAuthorName,
		"${siteAuthorId}", data.SiteAuthorID,
		"${siteWorkId}", data.SiteWorkID,
		"${siteWorkName}", data.SiteWorkName,
		"${description}", data.Description,
		"${uploadTimeYear}", data.UploadYear,
		"${uploadTimeMonth}", data.UploadMonth,
		"${uploadTimeDay}", data.UploadDay,
		"${uploadTimeHour}", data.UploadHour,
		"${uploadTimeMinute}", data.UploadMinute,
		"${uploadTimeSecond}", data.UploadSecond,
		"${downloadTimeYear}", data.DownloadYear,
		"${downloadTimeMonth}", data.DownloadMonth,
		"${downloadTimeDay}", data.DownloadDay,
		"${downloadTimeHour}", data.DownloadHour,
		"${downloadTimeMinute}", data.DownloadMinute,
		"${downloadTimeSecond}", data.DownloadSecond,
	)
	return r.Replace(tpl)
}

// --- 内部辅助函数 ---

func setDefaults(data *TokenData) {
	data.Author = fallbackAuthor
	data.LocalAuthorName = fallbackAuthor
	data.SiteAuthorName = fallbackAuthor
	data.SiteAuthorID = ""
}

func extractLocalAuthorName(authors []*sdkdto.LocalAuthorDTO) string {
	for _, a := range authors {
		if a.AuthorName != nil && *a.AuthorName != "" {
			return *a.AuthorName
		}
	}
	return fallbackAuthor
}

func extractSiteAuthorName(authors []*sdkdto.TaskSiteAuthorDTO) string {
	for _, a := range authors {
		if a.AuthorName != "" {
			return a.AuthorName
		}
	}
	return fallbackAuthor
}

// ptrStr 安全解引用 *string
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrStringValue 安全读取 WorkDTO 的指针字段
func ptrStringValue(work *sdkdto.WorkDTO, getter func(*sdkdto.WorkDTO) string) string {
	if work == nil {
		return ""
	}
	return getter(work)
}

// fillUploadTime 从 WorkDTO.SiteUploadTime（Unix 毫秒时间戳）提取时间组件
func fillUploadTime(data *TokenData, work *sdkdto.WorkDTO) {
	if work == nil || work.SiteUploadTime == nil || *work.SiteUploadTime == 0 {
		return
	}
	t := time.UnixMilli(*work.SiteUploadTime)
	data.UploadYear = fmt.Sprintf("%04d", t.Year())
	data.UploadMonth = fmt.Sprintf("%02d", t.Month())
	data.UploadDay = fmt.Sprintf("%02d", t.Day())
	data.UploadHour = fmt.Sprintf("%02d", t.Hour())
	data.UploadMinute = fmt.Sprintf("%02d", t.Minute())
	data.UploadSecond = fmt.Sprintf("%02d", t.Second())
}

// fillDownloadTime 使用当前时间填充下载时间组件
func fillDownloadTime(data *TokenData) {
	now := time.Now()
	data.DownloadYear = fmt.Sprintf("%04d", now.Year())
	data.DownloadMonth = fmt.Sprintf("%02d", now.Month())
	data.DownloadDay = fmt.Sprintf("%02d", now.Day())
	data.DownloadHour = fmt.Sprintf("%02d", now.Hour())
	data.DownloadMinute = fmt.Sprintf("%02d", now.Minute())
	data.DownloadSecond = fmt.Sprintf("%02d", now.Second())
}
