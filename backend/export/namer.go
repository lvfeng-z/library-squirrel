package export

import (
	"fmt"
	"path"
	"strings"

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
// 规则（方案第3节，风险3 对治）：sanitize(siteWorkName) → 空回退 sanitize(siteWorkId) →
// 再空回退 work_<id>；与已分配名冲突（不区分大小写，Windows 解压侧同名）追加序号 _2/_3。
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

// fileNamer 单个作品目录内的文件名分配器：优先保留原名，同目录同名冲突按 store 命名规约
// <bas>_<role>_<seq> 消解（声明8 复用），规约名仍冲突时追加序号。确定性：调用方按固定顺序逐个调用。
type fileNamer struct {
	used map[string]struct{}
}

func newFileNamer() *fileNamer {
	return &fileNamer{used: make(map[string]struct{})}
}

// Name 分配目标文件名。
// origName 源文件名（persistent_store.file_path 基名）；mount 提供 role/seq 消解锚；
// fallbackBas 原名净化为空时的兜底基底（作品目录名）。
func (n *fileNamer) Name(origName string, mount StoreMount, fallbackBas string) string {
	candidate := sanitizeComponent(origName)
	if candidate == "" {
		candidate = n.fallbackName(mount, fallbackBas)
	}
	if n.take(candidate) {
		return candidate
	}
	// 同名冲突：按 store 命名规约 <bas>_<role>_<seq> 消解
	bas, ext := splitNameExt(candidate)
	disambiguated := fmt.Sprintf("%s_%s_%03d%s", bas, mount.StoreType, mount.StoreSeq, ext)
	if disambiguated != candidate && n.take(disambiguated) {
		return disambiguated
	}
	// 规约名仍被占用（同 role 同 seq 重复挂载的极罕见场景）：追加序号
	base := disambiguated
	for i := 2; ; i++ {
		alt := fmt.Sprintf("%s_%d", base, i)
		if n.take(alt) {
			return alt
		}
	}
}

// fallbackName 原名净化为空时的兜底：<bas>_<role>_<seq>（bas 取净化后的作品目录名，再空则 file）。
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
// 确定性（方案第3节，决策3）：作品按 manifest 顺序（收集端按 ID 升序）逐个命名，
// 目录/文件冲突按上述固定规则消解——同输入同输出、结构可复现。
// 副作用：填充 manifest.Files[].Path。
func PlanNames(m *Manifest) error {
	fileIndex := make(map[int64]int, len(m.Files))
	for i, f := range m.Files {
		fileIndex[f.StoreID] = i
	}
	workNamer := newWorkDirNamer()
	for i := range m.Works {
		w := &m.Works[i]
		dirName := workNamer.Name(w.ID, w.SiteWorkName, w.SiteWorkID)
		fileNamer := newFileNamer()
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
				entry.Path = path.Join("works", dirName, fileNamer.Name(path.Base(entry.StorePath), mount, dirName))
			}
		}
	}
	return nil
}
