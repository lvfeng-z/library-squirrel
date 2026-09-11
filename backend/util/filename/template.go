package filename

import (
	"fmt"
	"strings"
	"time"
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
	ExportYear      string
	ExportMonth     string
	ExportDay       string
	ExportHour      string
	ExportMinute    string
	ExportSecond    string
}

// WorkFields 模板占位符值的中立入参：作者名列表 + 作品字段的扁平结构，
// 由消费方（导出命名等）按自身数据源组装，本包不感知任何上层 DTO
type WorkFields struct {
	LocalAuthorNames []string // 本地作者名（按作品关联顺序）
	SiteAuthorNames  []string // 站点作者名（按作品关联顺序）
	SiteAuthorID     string
	SiteWorkID       string
	SiteWorkName     string
	Description      string
	UploadTimeMs     int64 // 站点上传时间（Unix 毫秒）；0=未提供，uploadTime* 占位符留空
	ExportTimeMs     int64 // 导出时刻（Unix 毫秒），exportTime* 占位符的取值基准
}

const fallbackAuthor = "unknownAuthor"

// ExtractTokenData 从中立入参提取所有模板占位符的值
func ExtractTokenData(fields WorkFields) *TokenData {
	data := &TokenData{}

	// 作者名称
	data.LocalAuthorName = firstNonEmptyName(fields.LocalAuthorNames)
	data.SiteAuthorName = firstNonEmptyName(fields.SiteAuthorNames)
	data.SiteAuthorID = fields.SiteAuthorID

	// ${author}: 优先本地作者，其次站点作者
	if data.LocalAuthorName != fallbackAuthor {
		data.Author = data.LocalAuthorName
	} else {
		data.Author = data.SiteAuthorName
	}

	// 作品字段
	data.SiteWorkID = fields.SiteWorkID
	data.SiteWorkName = fields.SiteWorkName
	data.Description = fields.Description

	// 时间
	fillUploadTime(data, fields.UploadTimeMs)
	fillExportTime(data, fields.ExportTimeMs)

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
		"${exportTimeYear}", data.ExportYear,
		"${exportTimeMonth}", data.ExportMonth,
		"${exportTimeDay}", data.ExportDay,
		"${exportTimeHour}", data.ExportHour,
		"${exportTimeMinute}", data.ExportMinute,
		"${exportTimeSecond}", data.ExportSecond,
	)
	return r.Replace(tpl)
}

// --- 内部辅助函数 ---

// firstNonEmptyName 取列表首个非空名字；全空回退占位作者名
func firstNonEmptyName(names []string) string {
	for _, n := range names {
		if n != "" {
			return n
		}
	}
	return fallbackAuthor
}

// fillUploadTime 按 Unix 毫秒时间戳填充上传时间组件；0=未提供（组件留空）
func fillUploadTime(data *TokenData, ms int64) {
	if ms == 0 {
		return
	}
	t := time.UnixMilli(ms)
	data.UploadYear = fmt.Sprintf("%04d", t.Year())
	data.UploadMonth = fmt.Sprintf("%02d", t.Month())
	data.UploadDay = fmt.Sprintf("%02d", t.Day())
	data.UploadHour = fmt.Sprintf("%02d", t.Hour())
	data.UploadMinute = fmt.Sprintf("%02d", t.Minute())
	data.UploadSecond = fmt.Sprintf("%02d", t.Second())
}

// fillExportTime 按 Unix 毫秒时间戳填充导出时间组件
func fillExportTime(data *TokenData, ms int64) {
	t := time.UnixMilli(ms)
	data.ExportYear = fmt.Sprintf("%04d", t.Year())
	data.ExportMonth = fmt.Sprintf("%02d", t.Month())
	data.ExportDay = fmt.Sprintf("%02d", t.Day())
	data.ExportHour = fmt.Sprintf("%02d", t.Hour())
	data.ExportMinute = fmt.Sprintf("%02d", t.Minute())
	data.ExportSecond = fmt.Sprintf("%02d", t.Second())
}
