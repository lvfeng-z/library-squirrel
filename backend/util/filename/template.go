package filename

import (
	"fmt"
	"strings"
	"time"

	"github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
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
func ExtractTokenData(workResp *dto.WorkResponse) *TokenData {
	data := &TokenData{}

	if workResp == nil {
		setDefaults(data)
		fillDownloadTime(data)
		return data
	}

	// 作者名称
	data.LocalAuthorName = extractLocalAuthorName(workResp.LocalAuthors)
	data.SiteAuthorName = extractSiteAuthorName(workResp.SiteAuthors)
	data.SiteAuthorID = validString(workResp.Work, func(w *entity2.Work) string {
		return w.SiteAuthorID.String
	})

	// ${author}: 优先本地作者，其次站点作者
	if data.LocalAuthorName != fallbackAuthor {
		data.Author = data.LocalAuthorName
	} else {
		data.Author = data.SiteAuthorName
	}

	// 作品字段
	data.SiteWorkID = validString(workResp.Work, func(w *entity2.Work) string {
		return w.SiteWorkID.String
	})
	data.SiteWorkName = validString(workResp.Work, func(w *entity2.Work) string {
		return w.SiteWorkName.String
	})
	data.Description = validString(workResp.Work, func(w *entity2.Work) string {
		return w.SiteWorkDescription.String
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

func extractLocalAuthorName(authors []*dto.LocalAuthorDTO) string {
	for _, a := range authors {
		if a.AuthorName != nil && *a.AuthorName != "" {
			return *a.AuthorName
		}
	}
	return fallbackAuthor
}

func extractSiteAuthorName(authors []*dto.TaskSiteAuthorDTO) string {
	for _, a := range authors {
		if a.AuthorName != "" {
			return a.AuthorName
		}
	}
	return fallbackAuthor
}

// validString 安全读取 Work 的 sql.NullString 字段
func validString(work *entity2.Work, getter func(*entity2.Work) string) string {
	if work == nil {
		return ""
	}
	return getter(work)
}

// fillUploadTime 从 Work.SiteUploadTime（Unix 毫秒时间戳）提取时间组件
func fillUploadTime(data *TokenData, work *entity2.Work) {
	if work == nil || !work.SiteUploadTime.Valid || work.SiteUploadTime.Int64 == 0 {
		return
	}
	t := time.UnixMilli(work.SiteUploadTime.Int64)
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
