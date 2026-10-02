package importer

// 回灌前置编排的 manifest 子集构造与辅助（自 share 收件执行器提取，行为保持）：
// 子 manifest 构造（纯数据组装）、作品定位、板块角色集与文件跳过集。分享收件与
// zip 导入的任务粒度均为「父容器 + 每作品一子任务」，子任务回灌时先收窄到本作品子集。

import (
	"fmt"

	"github.com/library-squirrel/backend/export"
)

// BuildSubManifest 构造只含本作品的子 manifest（纯数据组装，不改 ManifestIngestor 接口）：
// Works 仅保留 manifestID 匹配的作品，Files 仅保留本作品 Stores[].StoreID 引用的条目；
// 站点/作者/标签/作品集保留全集（find-or-create 幂等，多子任务并发导入同一主数据无冲突）。
// Meta 计数按子 manifest 实际内容更新（仅展示用途，不参与导入判定）。
func BuildSubManifest(src *export.Manifest, manifestID int64) (*export.Manifest, error) {
	var work *export.WorkRecord
	for i := range src.Works {
		if src.Works[i].ID == manifestID {
			work = &src.Works[i]
			break
		}
	}
	if work == nil {
		return nil, fmt.Errorf("共享 manifest 中不存在作品 ID %d", manifestID)
	}
	storeIDs := make(map[int64]struct{})
	for i := range work.Resources {
		for _, s := range work.Resources[i].Stores {
			storeIDs[s.StoreID] = struct{}{}
		}
	}
	sub := &export.Manifest{
		SchemaVersion: src.SchemaVersion,
		Meta:          src.Meta,
		Sites:         src.Sites,
		LocalAuthors:  src.LocalAuthors,
		SiteAuthors:   src.SiteAuthors,
		LocalTags:     src.LocalTags,
		SiteTags:      src.SiteTags,
		WorkSets:      src.WorkSets,
		Works:         []export.WorkRecord{*work},
	}
	for i := range src.Files {
		if _, ok := storeIDs[src.Files[i].StoreID]; ok {
			sub.Files = append(sub.Files, src.Files[i])
		}
	}
	sub.Meta.WorkCount = 1
	sub.Meta.FileCount = len(sub.Files)
	return sub, nil
}

// findManifestWork 按 manifest 作品 ID 定位作品记录（子 manifest 通常单作品，通用化供多作品遍历）
func findManifestWork(manifest *export.Manifest, manifestID int64) *export.WorkRecord {
	for i := range manifest.Works {
		if manifest.Works[i].ID == manifestID {
			return &manifest.Works[i]
		}
	}
	return nil
}

// manifestWorkRoles 作品在 manifest 中声明的板块角色集合（资源挂载去重并集）：
// 作查重输入的期望板块与软删角色集（零交集命中时对活行 no-op）
func manifestWorkRoles(w *export.WorkRecord) []string {
	seen := make(map[string]struct{})
	var roles []string
	for i := range w.Resources {
		for _, s := range w.Resources[i].Stores {
			if s.StoreType == "" {
				continue
			}
			if _, dup := seen[s.StoreType]; dup {
				continue
			}
			seen[s.StoreType] = struct{}{}
			roles = append(roles, s.StoreType)
		}
	}
	return roles
}

// FileSkipSet 被裁决跳过作品的文件条目 StoreID 集：文件可能被多作品引用，
// 仅当引用它的全部作品都被跳过时才不获取（共享文件不因单作品跳过而丢失）
func FileSkipSet(manifest *export.Manifest, skipWorks map[int64]struct{}) map[int64]struct{} {
	if len(skipWorks) == 0 {
		return nil
	}
	workIDsByFile := make(map[int64]map[int64]struct{})
	for i := range manifest.Works {
		w := &manifest.Works[i]
		for j := range w.Resources {
			for _, s := range w.Resources[j].Stores {
				set := workIDsByFile[s.StoreID]
				if set == nil {
					set = make(map[int64]struct{})
					workIDsByFile[s.StoreID] = set
				}
				set[w.ID] = struct{}{}
			}
		}
	}
	skip := make(map[int64]struct{})
	for storeID, refs := range workIDsByFile {
		allSkipped := true
		for wid := range refs {
			if _, ok := skipWorks[wid]; !ok {
				allSkipped = false
				break
			}
		}
		if allSkipped {
			skip[storeID] = struct{}{}
		}
	}
	return skip
}
