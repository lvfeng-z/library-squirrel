//go:build windows

package fsmonitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/storeRegistry"
	"golang.org/x/sys/windows"
)

// fsctlReadFileUSNData FSCTL_READ_FILE_USN_DATA（winioctl.h）：经文件/目录句柄读其 USN_RECORD。
// 非管理员可用（仅需 FILE_READ_ATTRIBUTES 句柄），区别于卷级 FSCTL_READ_USN_JOURNAL（需管理员，R2）。
// CTL_CODE(FILE_DEVICE_FILE_SYSTEM=9, Function=58, METHOD_NEITHER=3, FILE_ANY_ACCESS=0) = 0x000900EB。
const fsctlReadFileUSNData = 0x000900EB

// readFRN 读取给定绝对路径（文件或目录）的文件引用号（FRN，NTFS 主文件表条目标识）。
// 经 FSCTL_READ_FILE_USN_DATA 读该路径的 USN_RECORD，取其 @8 低 64 位（V2/V3 同口径；
// V3 高 64 位在 NTFS 恒 0）。非管理员可用；返回的 FRN 用于 FRN→路径缓存构建。
func readFRN(absPath string) (uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return 0, fmt.Errorf("路径转 UTF-16 失败: %w", err)
	}
	// GENERIC_READ 非管理员对可读目录即可（R2 对照实验确认）；FILE_FLAG_BACKUP_SEMANTICS 使 CreateFile 能打开目录
	h, err := windows.CreateFile(
		pathPtr, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0,
	)
	if err != nil {
		return 0, fmt.Errorf("打开句柄失败: %w", err)
	}
	defer windows.CloseHandle(h)

	// 堆分配大缓冲：FSCTL_READ_FILE_USN_DATA 为 METHOD_NEITHER，驱动直接探测用户缓冲区，
	// 栈缓冲（var buf [N]byte）会触发 ERROR_INVALID_USER_BUFFER（C-0b PoC 踩坑，须堆分配）。
	// 输出直接是 USN_RECORD（无 READ_USN_JOURNAL 的 8 字节续读前缀）。
	buf := make([]byte, 65536)
	var returned uint32
	if err := windows.DeviceIoControl(h, fsctlReadFileUSNData, nil, 0, &buf[0], uint32(len(buf)), &returned, nil); err != nil {
		return 0, fmt.Errorf("读取 USN 记录失败: %w", err)
	}
	if returned < 16 {
		return 0, fmt.Errorf("USN 记录过短: %d 字节", returned)
	}
	return *(*uint64)(unsafe.Pointer(&buf[8])), nil
}

// frnPathCache 维护 workDir 白名单子树内「目录 FRN → workDir 相对路径（正斜杠）」映射。
//
// 用途：USN 记录路径解析（R1）。USN_RECORD 仅含 FileName + ParentFRN（父目录 FRN），不含全路径；
// 查缓存得父目录相对路径后拼 FileName 即得文件相对路径。仅缓存目录——路径解析只查 ParentFRN
// （恒为目录），文件 FRN 不入缓存，缓存体量小、构建快。
//
// 增量维护：随 USN 记录按提交序处理，目录新建/删除/移动时同步增删改缓存条目，使缓存始终反映
// 「已处理记录后的目录树状态」，后续记录解析用到的父目录处于正确的历史位置。
//
// 构建时机限制：缓存按当前文件系统状态（重启时刻 S1）构建，而 USN 记录描述离线期
// （保存游标时刻 S0 → S1）的变更。对窗口内未多次变更的目录（S0==S1 位置），解析正确；
// 对窗口内多次变更的目录，其首次重命名前的下级文件事件可能解析到最终位置——属可接受近似，
// 残余由全量对账兜底（D2）。
type frnPathCache struct {
	frnToRel map[uint64]string // 目录 FRN → workDir 相对路径（正斜杠）
	workDir  string            // workDir 绝对路径（构建遍历基准）
}

// newFrnPathCache 创建空的 FRN→路径缓存，须调用 Build 填充后使用。
func newFrnPathCache(workDir string) *frnPathCache {
	return &frnPathCache{
		frnToRel: make(map[uint64]string),
		workDir:  workDir,
	}
}

// Build 遍历白名单子树，逐目录读取 FRN（FSCTL_READ_FILE_USN_DATA）建立缓存。
// 单目录读失败（权限/非 NTFS）记日志跳过并继续；全部失败返回错误，调用方据此降级全量对账。
func (c *frnPathCache) Build(ctx context.Context) error {
	read := 0
	for _, sub := range storeRegistry.RegisteredPaths {
		absSub := filepath.Join(c.workDir, filepath.FromSlash(sub))
		info, err := os.Stat(absSub)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Log.Warnf("[fsmonitor] FRN 缓存：访问子目录失败 %s: %v", sub, err)
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.Walk(absSub, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !info.IsDir() {
				return nil
			}
			frn, err := readFRN(path)
			if err != nil {
				logger.Log.Warnf("[fsmonitor] FRN 缓存：读取目录 FRN 失败 %s: %v", path, err)
				return nil // 跳过此目录，继续遍历其下级
			}
			rel, err := filepath.Rel(c.workDir, path)
			if err != nil {
				return nil
			}
			c.frnToRel[frn] = filepath.ToSlash(rel)
			read++
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Log.Warnf("[fsmonitor] FRN 缓存：遍历子目录失败 %s: %v", sub, err)
		}
	}
	if read == 0 {
		return fmt.Errorf("FRN 缓存构建失败：未读取到任何目录 FRN（卷可能非 NTFS 或无 USN 能力）")
	}
	logger.Log.Infof("[fsmonitor] FRN→路径缓存构建完成：%d 个目录", read)
	return nil
}

// Resolve 解析 ParentFRN + FileName 为 workDir 相对路径（正斜杠）。
// ParentFRN 不在缓存（父目录在 workDir 子树外或已删除）→ ok=false，调用方据此丢弃外部变更。
func (c *frnPathCache) Resolve(parentFRN uint64, fileName string) (relPath string, ok bool) {
	parentRel, ok := c.frnToRel[parentFRN]
	if !ok {
		return "", false
	}
	return parentRel + "/" + fileName, true
}

// OnDirCreate 处理目录新建记录：父目录已缓存（在子树内）则把新目录加入缓存。
// 父目录未缓存（新建在子树外，或父目录尚未处理）→ 跳过。
func (c *frnPathCache) OnDirCreate(rec usnRecord) {
	parentRel, ok := c.frnToRel[rec.ParentFRN]
	if !ok {
		return
	}
	c.frnToRel[rec.FRN] = parentRel + "/" + rec.FileName
}

// OnDirDelete 处理目录删除记录：移除该目录及其所有下级目录的缓存条目（防 FRN 复用错映射，R6）。
// 该 FRN 不在缓存（本就外部）→ 无操作。
func (c *frnPathCache) OnDirDelete(rec usnRecord) {
	rel, ok := c.frnToRel[rec.FRN]
	if !ok {
		return
	}
	c.removeSubtree(rel)
}

// OnDirRename 处理目录移动/改名记录（old/new 已由上层按 FRN 配对，二者 FRN 相同）。
// 以缓存中该 FRN 的当前路径为源，整棵（含下级目录）迁移到「新父目录+新名」位置：
//   - 新父目录已缓存（仍在子树内）→ 迁移整棵到新路径（含下级前缀更新）；
//   - 新父目录未缓存（移出 workDir 子树）→ 若原本在缓存则整棵移除（归外部）；
//   - FRN 原不在缓存（外部移入）→ 作为新建加入（若新父目录已缓存）。
func (c *frnPathCache) OnDirRename(oldRec, newRec usnRecord) {
	newParentRel, newOk := c.frnToRel[newRec.ParentFRN]
	curRel, has := c.frnToRel[oldRec.FRN]
	if !newOk {
		// 目标在子树外：原本在缓存则整棵移除
		if has {
			c.removeSubtree(curRel)
		}
		return
	}
	newRel := newParentRel + "/" + newRec.FileName
	if !has {
		// 外部移入：作为新建加入
		c.frnToRel[oldRec.FRN] = newRel
		return
	}
	c.moveSubtree(curRel, newRel)
}

// moveSubtree 将缓存中路径为 oldPrefix 及其子树（oldPrefix/...）的条目整体迁移到 newPrefix。
// 用于目录移动/改名时同步其下级目录的路径前缀。
func (c *frnPathCache) moveSubtree(oldPrefix, newPrefix string) {
	for frn, rel := range c.frnToRel {
		switch {
		case rel == oldPrefix:
			c.frnToRel[frn] = newPrefix
		case strings.HasPrefix(rel, oldPrefix+"/"):
			c.frnToRel[frn] = newPrefix + rel[len(oldPrefix):]
		}
	}
}

// removeSubtree 移除缓存中路径为 prefix 及其子树（prefix/...）的条目。
// 用于目录删除/移出时清理下级目录条目，防止 FRN 复用错映射（R6）。
func (c *frnPathCache) removeSubtree(prefix string) {
	for frn, rel := range c.frnToRel {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			delete(c.frnToRel, frn)
		}
	}
}
