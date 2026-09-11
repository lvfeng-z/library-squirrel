package export

import (
	"fmt"
	"path"
	"strings"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/util/filename"
)

// maxNameComponentLen 导出包内单个路径组件（作品目录名 / 文件名）的最大长度（rune 数）。
// 防极端输入（超长作品名/文件名）撑爆整条 zip 路径，导致解压工具/文件系统拒绝。
// 取宽松上限，正常内容不受影响。
const maxNameComponentLen = 180

// windowsReservedNames Windows 保留设备名（不含扩展名比较；命中则前缀下划线避免解压冲突）。
// 覆盖 Windows 解压侧对设备名的保留规则：CON/PRN/AUX/NUL + COM1-9 + LPT1-9，带任意扩展名同样保留。
var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// sanitizeComponent 净化单个包内路径组件（作品目录名 / 文件名）。
// 非法字符替换为全角等价（复用既有 filename.SanitizeFileName 规约），
// 再处理 Windows 解压侧的特例：控制字符剔除、尾部点/空格去除、保留设备名前缀下划线、超长截断。
// 返回空串表示净化后无有效内容（调用方按命名规则回退）。
func sanitizeComponent(name string) string {
	name = filename.SanitizeFileName(name)
	// 剔除控制字符（含 \r\n\t 与其余 C0 控制码）
	name = strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return ""
	}
	// 保留设备名（含扩展名形式，如 CON.txt）前缀下划线，使其不再命中设备名规则
	base := strings.ToUpper(name)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	if _, reserved := windowsReservedNames[base]; reserved {
		name = "_" + name
	}
	// 超长截断（按 rune 截断避免切断多字节字符）
	if n := len([]rune(name)); n > maxNameComponentLen {
		name = string([]rune(name)[:maxNameComponentLen])
	}
	return name
}

// workDirNamer 作品目录名分配器：按固定顺序为每个作品分配唯一目录名。
// 规则：sanitize(siteWorkName) → 空回退 sanitize(siteWorkId) → 再空回退 work_<id>；
// 与已分配名冲突（不区分大小写，Windows 解压侧同名）追加序号 _2/_3。
// 同输入同输出：调用方按 manifest 作品顺序（收集端按 ID 升序）逐个调用。
type workDirNamer struct {
	used map[string]struct{}
}

func newWorkDirNamer() *workDirNamer {
	return &workDirNamer{used: make(map[string]struct{})}
}

// Name 为单个作品分配唯一目录名。
func (n *workDirNamer) Name(workID int64, siteWorkName *string, siteWorkID *string) string {
	candidate := ""
	if siteWorkName != nil {
		candidate = sanitizeComponent(*siteWorkName)
	}
	if candidate == "" && siteWorkID != nil {
		candidate = sanitizeComponent(*siteWorkID)
	}
	if candidate == "" {
		candidate = fmt.Sprintf("work_%d", workID)
	}
	base := candidate
	for i := 2; n.taken(candidate); i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	n.used[strings.ToLower(candidate)] = struct{}{}
	return candidate
}

func (n *workDirNamer) taken(name string) bool {
	_, ok := n.used[strings.ToLower(name)]
	return ok
}

// fileNamer 导出包内文件名分配器：唯一性按整个导出包全局消解（同包内同名文件可辨，
// 解压散落时不混淆）。主名 = 文件名模板渲染基底 + 源文件扩展名（渲染为空回退源文件名净化，
// 再空回退 <bas>_<role>_<seq>）；同名冲突追加 _siteWorkId 后缀消解（区分不同作品的同渲染名，
// 同作品多文件同扩展名同样经此步区分于首文件），仍冲突追加序号。确定性：调用方按固定顺序逐个调用。
type fileNamer struct {
	used map[string]struct{}
}

func newFileNamer() *fileNamer {
	return &fileNamer{used: make(map[string]struct{})}
}

// Name 分配目标文件名。
// origName 源文件名（persistent_store.file_path 基名）；mount 提供 role/seq 兜底锚；
// fallbackBas 渲染与原名均净化为空时的兜底基底（作品目录名）；renderedBase 模板渲染基底
// （已净化）；siteWorkID 冲突消解后缀素材。
func (n *fileNamer) Name(origName string, mount StoreMount, fallbackBas, renderedBase, siteWorkID string) string {
	base, ext := splitNameExt(origName)
	if renderedBase != "" {
		base = renderedBase
	} else if orig := sanitizeComponent(origName); orig != "" {
		// 模板渲染为空（模板为空/占位符全空值）：回退源文件名整体（扩展名已含在内）
		base, ext = orig, ""
	} else {
		base, ext = n.fallbackName(mount, fallbackBas), ""
	}
	if n.take(base + ext) {
		return base + ext
	}
	// 同名冲突：追加站点作品 ID 消歧（ID 为空时跳过直入序号）
	if siteWorkID != "" {
		base = base + "_" + siteWorkID
		if n.take(base + ext) {
			return base + ext
		}
	}
	// 仍冲突（同作品同扩展名多文件的第二份起）：追加序号
	for i := 2; ; i++ {
		alt := fmt.Sprintf("%s_%d%s", base, i, ext)
		if n.take(alt) {
			return alt
		}
	}
}

// fallbackName 渲染与原名均净化为空时的兜底：<bas>_<role>_<seq>（bas 取净化后的作品目录名，再空则 file）。
func (n *fileNamer) fallbackName(mount StoreMount, fallbackBas string) string {
	bas := sanitizeComponent(fallbackBas)
	if bas == "" {
		bas = "file"
	}
	return fmt.Sprintf("%s_%s_%03d", bas, mount.StoreType, mount.StoreSeq)
}

// take 占用一个文件名（不区分大小写）；已占用返回 false。
func (n *fileNamer) take(name string) bool {
	key := strings.ToLower(name)
	if _, taken := n.used[key]; taken {
		return false
	}
	n.used[key] = struct{}{}
	return true
}

// splitNameExt 拆分文件名基底与扩展名（扩展名含前导点；无扩展名或点开头则 base=原名, ext=""）。
func splitNameExt(name string) (base, ext string) {
	idx := strings.LastIndexByte(name, '.')
	if idx <= 0 {
		return name, ""
	}
	return name[:idx], name[idx:]
}

// PlanNames 为导出模型确定包内文件路径（works/<作品目录名>/<文件名>）。
// 文件名 = 文件名模板（fileNameFormat）按作品字段渲染的基底 + 源文件扩展名；目录名维持
// 作品名净化（siteWorkName → siteWorkId → work_<id>，冲突追加序号）。
// 确定性：作品按 manifest 顺序（收集端按 ID 升序）逐个命名，exportTime* 占位符基准取
// manifest 导出时刻（Meta.ExportedAt）——同 manifest 重复规划输出一致；文件名冲突按
// fileNamer 固定规则消解。副作用：填充 manifest.Files[].Path。
func PlanNames(m *Manifest, fileNameFormat string) error {
	fileIndex := make(map[int64]int, len(m.Files))
	for i, f := range m.Files {
		fileIndex[f.StoreID] = i
	}
	localAuthorNames := authorNameIndex(m.LocalAuthors)
	siteAuthorNames := authorNameIndex(m.SiteAuthors)
	workNamer := newWorkDirNamer()
	fileNamer := newFileNamer()
	for i := range m.Works {
		w := &m.Works[i]
		dirName := workNamer.Name(w.ID, w.SiteWorkName, w.SiteWorkID)
		token := filename.ExtractTokenData(workTemplateFields(m, w, localAuthorNames, siteAuthorNames))
		renderedBase := sanitizeComponent(filename.FormatFileName(fileNameFormat, token))
		siteWorkID := ptrStringValue(w.SiteWorkID)
		for r := range w.Resources {
			res := &w.Resources[r]
			for s := range res.Stores {
				mount := res.Stores[s]
				idx, ok := fileIndex[mount.StoreID]
				if !ok {
					continue // 挂载指向的文件条目缺失（数据异常，跳过命名）
				}
				entry := &m.Files[idx]
				if entry.Path != "" {
					continue // 同一文件被多挂载引用（数据异常）：首个分配为准
				}
				entry.Path = path.Join("works", dirName,
					fileNamer.Name(path.Base(entry.StorePath), mount, dirName, renderedBase, siteWorkID))
			}
		}
	}
	return nil
}

// workTemplateFields 组装单作品的模板入参：作者名按关联顺序解析（关联排序 local 前 site 后，
// 与 ${author} 本地优先语义一致），exportTime 基准取 manifest 导出时刻。
func workTemplateFields(m *Manifest, w *WorkRecord, localNames, siteNames map[int64]string) filename.WorkFields {
	fields := filename.WorkFields{
		SiteAuthorID: ptrStringValue(w.SiteAuthorID),
		SiteWorkID:   ptrStringValue(w.SiteWorkID),
		SiteWorkName: ptrStringValue(w.SiteWorkName),
		Description:  ptrStringValue(w.SiteWorkDescription),
		UploadTimeMs: ptrInt64Value(w.SiteUploadTime),
		ExportTimeMs: m.Meta.ExportedAt,
	}
	for _, link := range w.AuthorLinks {
		switch link.AuthorType {
		case constant.LOCAL:
			if name := localNames[link.AuthorID]; name != "" {
				fields.LocalAuthorNames = append(fields.LocalAuthorNames, name)
			}
		case constant.SITE:
			if name := siteNames[link.AuthorID]; name != "" {
				fields.SiteAuthorNames = append(fields.SiteAuthorNames, name)
			}
		}
	}
	return fields
}

// authorNameIndex 顶层作者记录 ID → 名字索引（空名不计入）。
func authorNameIndex(authors []AuthorRecord) map[int64]string {
	names := make(map[int64]string, len(authors))
	for _, a := range authors {
		if a.Name != nil && *a.Name != "" {
			names[a.ID] = *a.Name
		}
	}
	return names
}

func ptrStringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrInt64Value(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
