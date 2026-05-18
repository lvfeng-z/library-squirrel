package filename

import "strings"

// sanitizer 将文件系统非法字符替换为对应的全角字符
var sanitizer = strings.NewReplacer(
	`\`, "＼",
	"/", "／",
	":", "：",
	"*", "＊",
	"?", "？",
	`"`, "＂",
	"<", "＜",
	">", "＞",
	"|", "｜",
)

// SanitizeFileName 将文件名中的非法字符替换为全角等价字符
func SanitizeFileName(name string) string {
	return sanitizer.Replace(name)
}
