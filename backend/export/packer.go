package export

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// zipEntryTime zip 条目的写入时间：固定为导出时刻（manifest.meta.exportedAt）。
// zip 头默认嵌当前时间，写入时间不定则同输入同输出的字节级可复现不成立——固定时间保证确定性。
func zipEntryTime(exportedAt int64) time.Time {
	return time.UnixMilli(exportedAt)
}

// Packer 打包器：把导出数据模型（manifest + 源文件）按方案第3节布局写入 zip。
// 只做领域无关的文件编排（相对路径命名/流式写入/哈希），workDir 由调用方按需传入。
type Packer struct{}

// NewPacker 创建打包器。
func NewPacker() *Packer {
	return &Packer{}
}

// PackStats 打包规划统计（磁盘空间预检与进度总览共用）。
type PackStats struct {
	TotalFiles   int64 // 将写入 zip 的文件数（不含缺失）
	TotalBytes   int64 // 将写入的源文件总字节
	MissingFiles int64 // 源文件缺失标记数（决策4：跳过 + manifest 标注）
}

// ProgressFn 打包进度回调（每写入一个文件回调一次）：已处理文件数/累计字节 + 总文件数/总字节。
type ProgressFn func(processedFiles, processedBytes, totalFiles, totalBytes int64)

// Plan 打包规划：填充 FileEntry.Path（PlanNames）+ 检查源文件存在性/大小（决策4 分支语义），
// 返回统计供磁盘空间预检。不写文件，可重复调用（幂等——同输入同输出）。
// 缺失文件（决策4）：置 Missing=true 计入 MissingFiles，其余照常，不中断。
func (p *Packer) Plan(ctx context.Context, workDir string, model *ExportModel) (*PackStats, error) {
	if err := PlanNames(model.Manifest); err != nil {
		return nil, err
	}
	stats := &PackStats{}
	for i := range model.Manifest.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := &model.Manifest.Files[i]
		if entry.Path == "" {
			continue // 无包内路径（未被任何作品挂载引用，防御性跳过）
		}
		info, err := os.Stat(filepath.Join(workDir, entry.StorePath))
		if err != nil {
			if os.IsNotExist(err) {
				entry.Missing = true
				stats.MissingFiles++
				continue
			}
			return nil, fmt.Errorf("读取源文件信息失败 %s: %w", entry.StorePath, err)
		}
		if info.IsDir() {
			entry.Missing = true
			stats.MissingFiles++
			continue
		}
		entry.Size = info.Size()
		stats.TotalFiles++
		stats.TotalBytes += info.Size()
	}
	return stats, nil
}

// Pack 打包导出 zip 到 targetPath。依赖 Plan 已填充的 Path/Size/Missing（调用方先 Plan 预检，再 Pack 写入）。
// manifest.json deflate 压缩；媒体文件（其余全部条目）store 模式不压缩（风险5，大文件免重复压缩）。
// 逐文件流式写入并计算 sha256（回灌校验用）；onProgress 每写一个文件回调一次。
// 失败时不负责清理 targetPath（调用方 runner 持有临时文件生命周期）。
func (p *Packer) Pack(ctx context.Context, workDir string, model *ExportModel, targetPath string, stats *PackStats, onProgress ProgressFn) error {
	m := model.Manifest
	m.Meta.FileCount = len(m.Files)

	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("创建导出文件失败: %w", err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	entryTime := zipEntryTime(m.Meta.ExportedAt)
	var processedFiles, processedBytes int64

	// 媒体文件按 store 模式写入（不压缩），流式复制同时计算 sha256
	for i := range m.Files {
		entry := &m.Files[i]
		if entry.Path == "" || entry.Missing {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := os.Open(filepath.Join(workDir, entry.StorePath))
		if err != nil {
			return fmt.Errorf("打开源文件失败 %s: %w", entry.StorePath, err)
		}
		hdr := &zip.FileHeader{Name: entry.Path, Method: zip.Store}
		hdr.Modified = entryTime
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("创建 zip 条目失败 %s: %w", entry.Path, err)
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(w, hasher), src)
		closeErr := src.Close()
		if copyErr != nil {
			return fmt.Errorf("写入文件失败 %s: %w", entry.StorePath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("关闭源文件失败 %s: %w", entry.StorePath, closeErr)
		}
		entry.Size = written
		entry.Sha256 = hex.EncodeToString(hasher.Sum(nil))
		processedFiles++
		processedBytes += written
		if onProgress != nil {
			onProgress(processedFiles, processedBytes, stats.TotalFiles, stats.TotalBytes)
		}
	}

	// manifest.json deflate 压缩写入（含全部文件条目与填充后的 sha256/缺失标记）
	manifestData, err := m.Serialize()
	if err != nil {
		return fmt.Errorf("序列化 manifest 失败: %w", err)
	}
	mhdr := &zip.FileHeader{Name: "manifest.json", Method: zip.Deflate}
	mhdr.Modified = entryTime
	mw, err := zw.CreateHeader(mhdr)
	if err != nil {
		return fmt.Errorf("创建 manifest 条目失败: %w", err)
	}
	if _, err := mw.Write(manifestData); err != nil {
		return fmt.Errorf("写入 manifest 失败: %w", err)
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("关闭 zip 失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关闭导出文件失败: %w", err)
	}
	return nil
}
