package importer

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/export"
)

// manifestEntryName manifest 在导出包内的条目名（与 export.Packer 写入侧一致）。
const manifestEntryName = "manifest.json"

// Handler 导入 Handler（Wails Bind 方法，经 IPC 暴露给前端）。
type Handler struct {
	ingestor ManifestIngestor
}

// NewHandler 创建导入 Handler。
func NewHandler(ing ManifestIngestor) *Handler {
	return &Handler{ingestor: ing}
}

// ImportFromZip 从导出 ZIP 产物回灌导入：解包读 manifest → 校验版本锚（Ingest 内）→
// 入库 → 返回导入结果摘要。同步执行（进度/取消形态归二期任务模块接入，决策12）。
func (h *Handler) ImportFromZip(ctx context.Context, zipPath string) *model.ApiResponse[*ImportResult] {
	result, err := h.importFromZip(ctx, zipPath)
	if err != nil {
		return model.HandleError[*ImportResult](err)
	}
	return model.Success(result)
}

// importFromZip 打开导出产物、构建文件源并调用导入能力。
func (h *Handler) importFromZip(ctx context.Context, zipPath string) (*ImportResult, error) {
	if zipPath == "" {
		return nil, errors.New("导入产物路径为空")
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开导入产物失败: %w", err)
	}
	defer func() { _ = reader.Close() }()

	manifest, err := readManifest(&reader.Reader)
	if err != nil {
		return nil, err
	}
	return h.ingestor.Ingest(ctx, manifest, zipFileSource(&reader.Reader))
}

// readManifest 读取包内 manifest.json 并反序列化（版本锚校验由 Ingest 承担——能力契约对
// 全部消费方生效，非 zip 入口专属）。
func readManifest(r *zip.Reader) (*export.Manifest, error) {
	for _, f := range r.File {
		if f.Name != manifestEntryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("读取 manifest 失败: %w", err)
		}
		data, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读取 manifest 失败: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭 manifest 条目失败: %w", closeErr)
		}
		manifest, err := export.Deserialize(data)
		if err != nil {
			return nil, fmt.Errorf("解析 manifest 失败: %w", err)
		}
		return manifest, nil
	}
	return nil, fmt.Errorf("导入包缺少 %s", manifestEntryName)
}

// zipFileSource 构建包内路径 → 内容流的文件源（FileSource 的 zip 实现）。
func zipFileSource(r *zip.Reader) FileSource {
	index := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		index[f.Name] = f
	}
	return func(entryPath string) (io.ReadCloser, error) {
		f, ok := index[entryPath]
		if !ok {
			return nil, fmt.Errorf("%w：%s", ErrPackageFileMissing, entryPath)
		}
		return f.Open()
	}
}
