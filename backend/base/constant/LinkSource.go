package constant

// 作品关联（re_work_tag / re_work_author / re_work_work_set）的写入来源。
// PLUGIN=0 为零值：写入方未显式落来源（如导出包缺 source 字段的回灌）时缺省为插件来源。
const (
	PLUGIN = 0
	MANUAL = 1
)
