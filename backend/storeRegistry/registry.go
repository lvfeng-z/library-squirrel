package storeRegistry

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// StoreDir 存储目录：业务域注册进 persistent_store 代管的库内持久数据子树。
// 注册目录子树内的文件一律有 persistent_store 行（恒行支撑——参与指纹配对与缺失对账）；
// 域外目录（暂存根、备份根、守卫探针等）不进本表。
type StoreDir struct {
	// Path 子树根的 {workDir} 相对路径（正斜杠规范形，如 "store/work"），经 Register 登记后生效
	Path string
	// Owner 注册方模块标识（如 "resource"），用于审计、文档与注册冲突报错定位
	Owner string
}

// BackupDirPath 备份根目录（相对 {workDir}，正斜杠）。fsmonitor 的 backup 域监控根：
// 保管清单行（backup 表）的 file_path 全部落在该子树下。与存储目录注册表分立——
// backup 不参与 persistentStore 落盘校验（ValidatePath 仍拒绝 backup 路径），仅作监控范围谓词。
const BackupDirPath = "backup"

// 注册失败哨兵：Register 返回的错误包裹本组哨兵，调用方以 errors.Is 区分失败类别
var (
	// ErrInvalidDirPath 路径非法：空串、反斜杠、绝对路径、非规范形或越出 workDir。
	ErrInvalidDirPath = errors.New("存储目录路径非法")
	// ErrEmptyOwner 注册方模块标识（Owner）为空。
	ErrEmptyOwner = errors.New("存储目录注册方标识为空")
	// ErrDuplicateDir 与已注册目录路径相同（重复注册）。
	ErrDuplicateDir = errors.New("存储目录重复注册")
	// ErrPrefixConflict 与已注册目录互为父子前缀（子树重叠）。
	ErrPrefixConflict = errors.New("存储目录前缀冲突")
)

var (
	registryMu sync.RWMutex
	registry   []StoreDir
)

// Register 注册一个存储目录，成功后 ValidatePath / InScanDirs / RegisteredDirs 即认该子树。
// 调用时机：装配期、fsmonitor 启动前——对账扫描根与 USN 缓存种子在监控启动时取注册快照，
// 晚于监控启动的注册对既取快照不可见。前缀冲突与路径非法属装配错误，调用方 fail-fast 阻断启动。
func Register(dir StoreDir) error {
	if dir.Owner == "" {
		return fmt.Errorf("%w: %q", ErrEmptyOwner, dir.Path)
	}
	cleaned := path.Clean(dir.Path)
	if dir.Path == "" || strings.Contains(dir.Path, `\`) || path.IsAbs(dir.Path) ||
		cleaned != dir.Path || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: %q（须为 workDir 相对、正斜杠、无冗余段与尾斜杠、不越出 workDir）",
			ErrInvalidDirPath, dir.Path)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Path == cleaned {
			return fmt.Errorf("%w: %q 已由 %s 注册", ErrDuplicateDir, cleaned, existing.Owner)
		}
		if isSubtreeOf(cleaned, existing.Path) || isSubtreeOf(existing.Path, cleaned) {
			return fmt.Errorf("%w: %q 与 %s 注册的 %q 子树重叠",
				ErrPrefixConflict, cleaned, existing.Owner, existing.Path)
		}
	}
	registry = append(registry, StoreDir{Path: cleaned, Owner: dir.Owner})
	return nil
}

// RegisteredDirs 返回已注册存储目录的快照（切片独立分配、元素为值拷贝，
// 调用方持有或修改不影响注册表）。启动期消费方（对账扫描根、USN 缓存种子）取本快照遍历。
func RegisteredDirs() []StoreDir {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]StoreDir, len(registry))
	copy(out, registry)
	return out
}

// RegisteredPaths 已注册存储目录的路径列表（RegisteredDirs 派生，供仅需路径的遍历消费）。
func RegisteredPaths() []string {
	dirs := RegisteredDirs()
	paths := make([]string, len(dirs))
	for i, d := range dirs {
		paths[i] = d.Path
	}
	return paths
}

// isSubtreeOf 判断 sub 是否位于 ancestor 子树内（含相等）。按 "/" 段边界匹配，
// "store/workX" 不被 "store/work" 误含。
func isSubtreeOf(sub, ancestor string) bool {
	return sub == ancestor || strings.HasPrefix(sub, ancestor+"/")
}

// matchRegistered 判断规范化后的 workDir 相对路径是否命中任一已注册子树（含子树根自身）。
func matchRegistered(normalized string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, d := range registry {
		if isSubtreeOf(normalized, d.Path) {
			return true
		}
	}
	return false
}

// ValidatePath 校验路径是否以已注册子目录开头（落盘前路径校验）。
// relPath 如 "store/work/作者/video.mp4"、"store/avatar/local/123.jpg"。
// 内部统一转正斜杠比较，兼容 Windows 下 filepath.Join 产出的反斜杠路径。
func ValidatePath(relPath string) error {
	if !matchRegistered(filepath.ToSlash(relPath)) {
		return fmt.Errorf("路径 %q 未匹配任何已注册子目录", relPath)
	}
	return nil
}

// InScanDirs 判断 workDir 相对路径是否命中任一已注册子树（含子树根自身）。
// 离线对账扫描与 USN 路径过滤共用：变更路径须命中白名单才纳入，
// store/ 与 backup/ 之外的变更（.git/ 等）为噪声丢弃。rel 用正斜杠基准（与 file_path 一致）。
func InScanDirs(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	return matchRegistered(filepath.ToSlash(rel))
}

// InBackupDir 判断 workDir 相对路径是否命中备份根子树（含根自身 backup）。
// fsmonitor 的 backup 域事件路由与 USN 路径过滤共用（与 InScanDirs 的 store 域口径分立）。
// rel 用正斜杠基准（与 backup.file_path 一致）。
func InBackupDir(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == BackupDirPath || strings.HasPrefix(rel, BackupDirPath+"/")
}
