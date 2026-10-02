package importer

// 回灌前置编排的逐文件内容判定与本地拷贝（自 share 收件执行器提取，行为保持）：
// 对确认替换作品按 manifest 文件条目与本地活行 store 做两级比对（头部指纹直比 +
// 全量哈希强校验），全部一致整作品静默跳过（幂等保障），部分一致的匹配文件由本地
// 活文件拷入暂存免重复获取（分享收件=免网络重拉、zip 导入=免解包重读）。
// 两级比对维持「缺指纹=不匹配」现状语义，不做慢路径退化——旧版导出包经版本锚
// 整体拒绝，到达内容判定的包必有指纹。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/resource"
)

// mountKey 挂载键（store_type + store_seq）：manifest 文件条目与本地活行 store 按此配对判定
type mountKey struct {
	storeType string
	storeSeq  int64
}

// applyContentMatches 对确认替换作品做逐文件内容判定：
//   - 全部文件与本地活行 store 内容一致（头部指纹快路径 + 全量 sha256 强校验）→ 整作品跳过：
//     从确认替换集移除并加入整作品跳过集——不软删/不获取/不导入，保留现状（confirmTargets 同步过滤）。
//   - 部分匹配 → 作品保留确认替换（整作品替换收尾），匹配文件由本地活文件拷入暂存免重复获取
//     （获取侧见「暂存==声明」即跳过）。
//   - 无匹配/缺指纹/无本地对应 store/文件数 0 → 原样整作品替换（安全回退，不误跳过）。
//
// 判定仅作用于确认替换目标；裁决跳过作品已入 SkipWorks、零交集自动增补不涉及覆盖，均不受影响。
func (p *ReplacePlanner) applyContentMatches(ctx context.Context, manifest *export.Manifest,
	staging, workDir string, plan *ReplacePlan, targets *[]replaceSoftTarget) error {
	workIds := make([]int64, 0, len(*targets))
	for _, t := range *targets {
		if _, ok := plan.ConfirmedWorks[t.manifestID]; ok {
			workIds = append(workIds, t.localWorkID)
		}
	}
	localMounts, err := p.mountReader.ListMountsByWorkIds(ctx, workIds)
	if err != nil {
		return err
	}
	entryByStoreID := make(map[int64]*export.FileEntry, len(manifest.Files))
	for i := range manifest.Files {
		entryByStoreID[manifest.Files[i].StoreID] = &manifest.Files[i]
	}
	for i := len(*targets) - 1; i >= 0; i-- {
		t := (*targets)[i]
		if _, ok := plan.ConfirmedWorks[t.manifestID]; !ok {
			continue // 裁决跳过作品已入 SkipWorks，不参与内容判定
		}
		work := findManifestWork(manifest, t.manifestID)
		if work == nil {
			continue
		}
		fileByKey := make(map[mountKey]*export.FileEntry)
		for j := range work.Resources {
			for _, s := range work.Resources[j].Stores {
				fileByKey[mountKey{s.StoreType, int64(s.StoreSeq)}] = entryByStoreID[s.StoreID]
			}
		}
		localByKey := make(map[mountKey]resource.StoreMountInfo)
		for _, m := range localMounts[t.localWorkID] {
			localByKey[mountKey{m.StoreType, m.StoreSeq}] = m
		}
		var matched []resource.StoreMountInfo
		total := 0
		for key, entry := range fileByKey {
			if entry == nil {
				continue // 挂载无文件条目（异常态）：不计入文件数，不影响判定
			}
			total++
			if entry.Missing || entry.Path == "" {
				continue // 缺席/无包内路径文件不参与匹配（也不获取），同时阻断整作品跳过
			}
			local, ok := localByKey[key]
			if !ok {
				continue // 无本地对应活行 store → 不匹配
			}
			if p.fileContentMatches(ctx, workDir, local, entry) {
				matched = append(matched, local)
			}
		}
		if total > 0 && len(matched) == total {
			// 全部匹配：内容一致 → 整作品跳过（保留现状，不软删/不获取/不导入）
			delete(plan.ConfirmedWorks, t.manifestID)
			plan.SkipWorks[t.manifestID] = struct{}{}
			*targets = append((*targets)[:i], (*targets)[i+1:]...)
			continue
		}
		if len(matched) == 0 {
			continue // 无匹配 → 原样整作品替换（安全回退）
		}
		// 部分匹配：匹配文件拷入暂存（免重复获取）；不匹配文件留待获取侧处理
		for _, local := range matched {
			key := mountKey{local.StoreType, local.StoreSeq}
			if err := copyLocalToStaging(ctx, staging, workDir, local, fileByKey[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

// fileContentMatches 判定本地活文件与宿主文件条目内容一致：快路径头部指纹直比（零读盘），
// 匹配再读本地文件全量 sha256 与 manifest Sha256 比对（全量强校验）。
// 任一指纹缺失/本地路径为空/本地文件不可读 → 不匹配（安全回退，不误跳过）。
func (p *ReplacePlanner) fileContentMatches(ctx context.Context, workDir string,
	local resource.StoreMountInfo, entry *export.FileEntry) bool {
	if entry.ContentFingerprint == "" || entry.Sha256 == "" || local.ContentFingerprint == "" || local.StorePath == "" {
		return false
	}
	if entry.ContentFingerprint != local.ContentFingerprint {
		return false
	}
	sum, err := sha256File(ctx, filepath.Join(workDir, filepath.FromSlash(local.StorePath)))
	if err != nil {
		return false
	}
	return sum == entry.Sha256
}

// copyLocalToStaging 把本地活文件拷入暂存（满尺寸；获取侧见「暂存==声明」即跳过）。
// 拷贝读流 tee 全量 sha256 二次确认——源文件在判定与拷贝之间被改动则放弃拷贝改回源获取。
// 所有失败路径移除半成品暂存后回落原源获取（不阻断整作品替换主流程，真实错误由获取侧报出）。
func copyLocalToStaging(ctx context.Context, staging, workDir string,
	local resource.StoreMountInfo, entry *export.FileEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	target := StagingPath(staging, entry.Path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil // 暂存不可写由获取侧报错，此处回落原源获取
	}
	src, err := os.Open(filepath.Join(workDir, filepath.FromSlash(local.StorePath)))
	if err != nil {
		return nil // 本地文件已不可读 → 回落原源获取
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = os.Remove(target)
		return nil
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(dst, io.TeeReader(src, hasher))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(target)
		return nil
	}
	if written != entry.Size || hex.EncodeToString(hasher.Sum(nil)) != entry.Sha256 {
		// 源文件内容在判定后漂移（size 或全量哈希不一致）：放弃拷贝，改由获取侧从原源获取
		_ = os.Remove(target)
		return nil
	}
	return nil
}

// sha256File 计算文件全量 SHA256（hex；读本地活文件作内容身份强校验）
func sha256File(ctx context.Context, absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// StagingPath 暂存内绝对路径（包内路径正斜杠 → 平台分隔符；absPath 域，仅 os 调用点使用）。
// 分享收件的拉取暂存与导入的解包暂存共用同一「包内路径 → 暂存落点」映射规则。
func StagingPath(staging, entryPath string) string {
	return filepath.Join(staging, filepath.FromSlash(entryPath))
}
