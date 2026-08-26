package export

// ExportModel 导出收集结果（内存态导出模型）。
// 阶段2 数据面产物：覆盖方案第2节全部数据域，供阶段3（ZIP 打包）消费。
// Manifest 为可序列化契约；fileIndexByStoreID 为打包阶段的源文件查找索引（不参与序列化）。
type ExportModel struct {
	Manifest *Manifest `json:"manifest"`

	// fileIndexByStoreID 源 persistent_store 行 ID → files[] 索引（打包阶段按源路径读文件用）
	fileIndexByStoreID map[int64]int
}

// NewExportModel 创建导出模型。
func NewExportModel(manifest *Manifest) *ExportModel {
	index := make(map[int64]int, len(manifest.Files))
	for i, f := range manifest.Files {
		index[f.StoreID] = i
	}
	return &ExportModel{
		Manifest:           manifest,
		fileIndexByStoreID: index,
	}
}

// FileIndexByStoreID 按源 persistent_store 行 ID 查 files[] 索引。
func (m *ExportModel) FileIndexByStoreID(storeID int64) (int, bool) {
	idx, ok := m.fileIndexByStoreID[storeID]
	return idx, ok
}
