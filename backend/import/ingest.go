// Package importer 导出产物回灌导入（分享功能方案阶段2，即导出方案阶段4）。
// 读取导出 manifest（契约复用 backend/export，禁止重定义）+ 包内文件，完整重建作品与关联；
// 入库逻辑抽为 ManifestIngestor 能力接口，供导出回灌（本模块 Handler）与分享收件人侧
// （share-receive 任务执行器，二期）复用。
package importer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/duplicate"
	"github.com/library-squirrel/backend/export"
	"github.com/library-squirrel/backend/storeRegistry"
	"github.com/library-squirrel/backend/util"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// ===== 导入错误定义 =====

var (
	// ErrSchemaVersionUnsupported manifest 版本锚不匹配（契约破坏性变更防护，对齐 export.SchemaVersion 纪律）
	ErrSchemaVersionUnsupported = errors.New("导入产物 manifest 版本不支持")
	// ErrUnregisteredSiteKey manifest 站点记录携带未注册的站点键（含空键）——站点身份键由
	// SDK identity 注册表统一分配，未注册键的产物来源不可信，整体拒绝导入
	ErrUnregisteredSiteKey = errors.New("导入产物包含未注册的站点键")
	// ErrUnsafeStorePath 落盘目标路径不合法（路径穿越 / 反斜杠 / 未注册子目录）
	ErrUnsafeStorePath = errors.New("导入文件路径不合法")
	// ErrPackageFileMissing 包内缺少 manifest 声明的文件
	ErrPackageFileMissing = errors.New("导入包缺少声明的文件")
	// ErrChecksumMismatch 文件内容与 manifest 记录的 sha256 不一致
	ErrChecksumMismatch = errors.New("导入文件校验失败")
	// ErrNoFileSource 产物包含待落盘文件但未提供文件源
	ErrNoFileSource = errors.New("导入产物包含文件但未提供文件源")
)

// FileSource 导出产物文件内容源：按 manifest files[].Path（包内相对路径）提供内容流。
// 导出回灌由 zip 打开器实现；分享收件侧由网络拉取流实现——导入核心不感知来源形态。
type FileSource func(entryPath string) (io.ReadCloser, error)

// IngestOptions 回灌导入选项（nil = 现状全跳过语义，zip 导入沿用）。
// 替换全集 = ReplaceWorks ∪ AutoMergeWorks（二者皆空 = 不替换任何已存在作品）。
type IngestOptions struct {
	// ReplaceWorks 经用户确认要替换的 manifest 作品 ID 集（冲突确认替换：角色交集命中、
	// 用户选择替换后由调用方传入）。命中该集的作品走替换分支——资源面新建挂载、
	// 元数据覆盖、关联合并、空壳净化。
	ReplaceWorks map[int64]struct{}
	// AutoMergeWorks 零交集命中自动并入的 manifest 作品 ID 集（不经用户确认直接增补挂载到
	// 已有作品——查重命中即挂已有作品、弹窗与否只决定用户确认的既有语义）。统计按
	// 「确认替换 / 自动增补」拆分计数（ReplacedConfirmed / ReplacedAuto）。
	AutoMergeWorks map[int64]struct{}
}

// ManifestIngestor manifest 驱动回灌导入能力（分享收件人侧任务执行器复用的能力形态）：
// 校验版本锚、find-or-create 站点/标签/作者、按 site_id+site_work_id 查重作品、
// 导出库 ID → 本库 ID 重映射、文件落盘与全量关联重建。
type ManifestIngestor interface {
	// Ingest 导入一份导出 manifest（文件内容经 fileSource 按包内路径读取），返回结果摘要。
	// opts 为 nil 时行为与现签名完全一致：已存在作品整体跳过；opts 携带替换集时命中作品
	// 走替换分支（资源面新建挂载、元数据覆盖、关联合并、空壳净化，见 IngestOptions 注释）。
	Ingest(ctx context.Context, manifest *export.Manifest, fileSource FileSource, opts *IngestOptions) (*ImportResult, error)
}

// Transactor 事务执行器（事务 DB 经 ctx 传递；app.go 以 dbTransactorAdapter 装配）。
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// FileStoreOperator 导入文件落盘能力（由 persistentStore.Service 实现）：
// 文件 + persistent_store 记录同写（含 fsmonitor 操作抑制登记、内容指纹、图片宽高提取）、
// 按路径查活行记录（落位冲突检测）、物理删记录与文件（导入失败补偿清理）。
type FileStoreOperator interface {
	Store(ctx context.Context, relPath string, fileName string, reader io.Reader) (int64, error)
	GetByFilePath(ctx context.Context, filePath string) (*entity.PersistentStore, error)
	HardDelete(ctx context.Context, id int64, backup bool) (int64, error)
}

// ImportResult 导入结果摘要。
type ImportResult struct {
	CreatedWorks        int64 `json:"createdWorks"`        // 新建作品数
	ReplacedWorks       int64 `json:"replacedWorks"`       // 替换作品数（确认替换 + 自动增补 全集）
	ReplacedConfirmed   int64 `json:"replacedConfirmed"`   // 经用户确认替换的作品数
	ReplacedAuto        int64 `json:"replacedAuto"`        // 零交集命中自动增补的作品数
	SkippedWorks        int64 `json:"skippedWorks"`        // 查重命中（site_id+site_work_id 已存在）跳过的作品数
	CreatedWorkSets     int64 `json:"createdWorkSets"`     // 新建作品集数
	SkippedWorkSets     int64 `json:"skippedWorkSets"`     // 查重命中跳过的作品集数
	CreatedSites        int64 `json:"createdSites"`        // 新建站点数（按键 find-or-create）
	CreatedLocalTags    int64 `json:"createdLocalTags"`    // 新建本地标签数（按名称 find-or-create）
	CreatedSiteTags     int64 `json:"createdSiteTags"`     // 新建站点标签数（按站点+site_tag_id 匹配）
	CreatedLocalAuthors int64 `json:"createdLocalAuthors"` // 新建本地作者数（按名称 find-or-create）
	CreatedSiteAuthors  int64 `json:"createdSiteAuthors"`  // 新建站点作者数（按站点+site_author_id 匹配）
	ExtractedFiles      int64 `json:"extractedFiles"`      // 落盘文件数
	AbsentStores        int64 `json:"absentStores"`        // 挂载缺席数（源文件缺失/无包内路径，决策4）
}

// ingestor ManifestIngestor 实现。
type ingestor struct {
	repo       Repository
	dupRepo    duplicate.Repository // 站点名/作品查重共享查询（自本模块迁出，与 duplicate.Service 共用同一实例）
	transactor Transactor
	fileStore  FileStoreOperator
}

// NewIngestor 创建导入器。
func NewIngestor(dupRepo duplicate.Repository, repo Repository, transactor Transactor, fileStore FileStoreOperator) ManifestIngestor {
	return &ingestor{
		repo:       repo,
		dupRepo:    dupRepo,
		transactor: transactor,
		fileStore:  fileStore,
	}
}

// filePhaseResult 文件相位产物。
type filePhaseResult struct {
	storeRemap     map[int64]int64 // 导出库 persistent_store ID → 本库新记录 ID
	createdStores  []int64         // 本次导入创建的 persistent_store ID（失败补偿清理用）
	extractedFiles int64
}

// Ingest 导入主流程，三相推进：
//
//	相位一（只读预检）：既有站点映射 + 作品查重圈定待建集（决定哪些文件需要落盘）
//	相位二（文件落盘）：为待建作品的 store 挂载提取包内文件并经 persistentStore 能力落盘
//	相位三（单事务入库）：find-or-create 主数据 + 新建作品/资源/挂载/关联 + 全量 ID 重映射
//
// 相位二先于事务（文件 IO 不持有唯一 DB 连接），失败时对已落盘文件做补偿清理。
func (ing *ingestor) Ingest(ctx context.Context, manifest *export.Manifest, fileSource FileSource, opts *IngestOptions) (*ImportResult, error) {
	if manifest == nil {
		return nil, errors.New("导入 manifest 为空")
	}
	if manifest.SchemaVersion != export.SchemaVersion {
		return nil, fmt.Errorf("%w：产物版本 %d，本程序支持 %d",
			ErrSchemaVersionUnsupported, manifest.SchemaVersion, export.SchemaVersion)
	}

	// 站点键注册校验（入口前置，文件落盘与入库前失败）：manifest 站点记录的 site_key 须全部
	// 在 identity 注册表内，未注册键（含空键）整体拒绝
	if err := validateSiteKeys(manifest.Sites); err != nil {
		return nil, err
	}

	// 相位一：作品查重圈定（查重键的站点侧需要既有站点映射解析）
	preSiteRemap, err := ing.mapExistingSites(ctx, manifest.Sites)
	if err != nil {
		return nil, err
	}
	replaceSet := replaceUnion(opts)
	existingWorks, toCreateWorks, toReplaceWorks, err := ing.partitionWorks(ctx, manifest.Works, preSiteRemap, replaceSet)
	if err != nil {
		return nil, err
	}

	// 相位二：文件落盘（待建与替换作品；其余查重命中的作品其文件与记录一概不动）
	toExtract := make([]*export.WorkRecord, 0, len(toCreateWorks)+len(toReplaceWorks))
	toExtract = append(toExtract, toCreateWorks...)
	toExtract = append(toExtract, toReplaceWorks...)
	filePhase, err := ing.extractFiles(ctx, manifest, toExtract, fileSource)
	if err != nil {
		ing.cleanupCreatedStores(ctx, filePhase.createdStores)
		return nil, err
	}

	// 相位三：单事务入库
	var result *ImportResult
	err = ing.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		var txErr error
		result, txErr = ing.ingestEntities(txCtx, manifest, toCreateWorks, toReplaceWorks, existingWorks, opts, filePhase.storeRemap)
		return txErr
	})
	if err != nil {
		ing.cleanupCreatedStores(ctx, filePhase.createdStores)
		return nil, err
	}
	result.ExtractedFiles = filePhase.extractedFiles
	logger.Log.Infof("导入完成：新建作品 %d / 替换 %d（确认 %d / 增补 %d）/ 跳过 %d，新建作品集 %d / 跳过 %d，落盘文件 %d",
		result.CreatedWorks, result.ReplacedWorks, result.ReplacedConfirmed, result.ReplacedAuto, result.SkippedWorks,
		result.CreatedWorkSets, result.SkippedWorkSets, result.ExtractedFiles)
	return result, nil
}

// replaceUnion 计算替换全集（确认替换 ∪ 零交集自动增补）；opts 为空或两集皆空返回 nil
// （现状全跳过语义）。
func replaceUnion(opts *IngestOptions) map[int64]struct{} {
	if opts == nil {
		return nil
	}
	if len(opts.ReplaceWorks) == 0 && len(opts.AutoMergeWorks) == 0 {
		return nil
	}
	set := make(map[int64]struct{}, len(opts.ReplaceWorks)+len(opts.AutoMergeWorks))
	for id := range opts.ReplaceWorks {
		set[id] = struct{}{}
	}
	for id := range opts.AutoMergeWorks {
		set[id] = struct{}{}
	}
	return set
}

// ===== 相位一：只读预检 =====

// validateSiteKeys 校验 manifest 站点记录的键全部在 identity 注册表内（空键/未注册键报错，
// 报错文案即注册渠道指引）。键已注册方可继续——后续 find-or-create 以注册表权威信息建行。
func validateSiteKeys(records []export.SiteRecord) error {
	for _, r := range records {
		if _, ok := identity.Lookup(r.SiteKey); !ok {
			return fmt.Errorf("%w：%q。站点身份键由 SDK identity 注册表统一分配，"+
				"新站点请生成 24 位小写 hex 强随机键值并提 PR 至 github.com/lvfeng-z/library-squirrel-sdk 的 identity 包注册，随 SDK 发布生效",
				ErrUnregisteredSiteKey, r.SiteKey)
		}
	}
	return nil
}

// mapExistingSites 既有站点行映射（站点按键匹配：site_key 为站点唯一身份）。
// 只读预检，供作品查重键解析站点侧；真正的 find-or-create 在入库事务内完成（ensureSites）。
func (ing *ingestor) mapExistingSites(ctx context.Context, records []export.SiteRecord) (map[int64]int64, error) {
	keys := make([]string, 0, len(records))
	for _, r := range records {
		keys = append(keys, r.SiteKey)
	}
	rows, err := ing.dupRepo.ListSitesByKeys(ctx, util.UniqueString(keys))
	if err != nil {
		return nil, fmt.Errorf("查询既有站点失败: %w", err)
	}
	keyToLocalID := make(map[string]int64, len(rows))
	for _, row := range rows {
		keyToLocalID[row.SiteKey] = row.GetID()
	}
	remap := make(map[int64]int64, len(records))
	for _, r := range records {
		if localID, ok := keyToLocalID[r.SiteKey]; ok {
			remap[r.ID] = localID
		}
	}
	return remap, nil
}

// partitionWorks 作品查重圈定（决策15：site_id+site_work_id 联合键，跨库稳定身份）。
// 键不完整（站点或站点作品 ID 缺失）或站点在本库不存在（不可能有作品引用它）时判待建；
// 无站点身份的作品无跨库稳定身份，恒判待建——重复导入会重复建，属查重语义兜底范围之外
// （与无站点身份作品集同一口径）。
// 查重命中的作品按 replaceSet 分流：命中者入 toReplace（替换分支），未命中者保持仅记录于
// existing（跳过语义）。replaceSet 为 nil 时 toReplace 恒空，行为与旧签名一致。
func (ing *ingestor) partitionWorks(ctx context.Context, works []export.WorkRecord, siteRemap map[int64]int64, replaceSet map[int64]struct{}) (existing map[int64]int64, toCreate []*export.WorkRecord, toReplace []*export.WorkRecord, err error) {
	type workKey struct {
		siteID     int64
		siteWorkID string
	}
	existing = make(map[int64]int64, len(works))
	// 按本库站点分组批量查（site_id+site_work_id 等长配对语义）
	keysByLocalSite := make(map[int64][]workKey)
	for i := range works {
		w := &works[i]
		if w.SiteID == nil || w.SiteWorkID == nil || *w.SiteWorkID == "" {
			continue
		}
		localSiteID, ok := siteRemap[*w.SiteID]
		if !ok {
			continue
		}
		keysByLocalSite[localSiteID] = append(keysByLocalSite[localSiteID], workKey{localSiteID, *w.SiteWorkID})
	}
	found := make(map[workKey]int64)
	for localSiteID, keys := range keysByLocalSite {
		ids := make([]string, 0, len(keys))
		for _, k := range keys {
			ids = append(ids, k.siteWorkID)
		}
		rows, err := ing.dupRepo.ListWorksBySiteAndWorkIDs(ctx, localSiteID, ids)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("查询既有作品失败: %w", err)
		}
		for _, row := range rows {
			if row.SiteID.Valid && row.SiteWorkID.Valid {
				found[workKey{row.SiteID.Int64, row.SiteWorkID.String}] = row.GetID()
			}
		}
	}
	for i := range works {
		w := &works[i]
		if w.SiteID != nil && w.SiteWorkID != nil {
			if localID, hit := found[workKey{siteRemap[*w.SiteID], *w.SiteWorkID}]; hit {
				existing[w.ID] = localID
				if _, inReplace := replaceSet[w.ID]; inReplace {
					toReplace = append(toReplace, w)
				}
				continue
			}
		}
		toCreate = append(toCreate, w)
	}
	return existing, toCreate, toReplace, nil
}

// ===== 相位二：文件落盘 =====

// extractFiles 为待建与替换作品的 store 挂载提取包内文件并落盘。
// 结构校验（ResourceType/StoreType/Generation 严格识别）前置到本相位开头——非法产物在写盘前失败。
// 挂载缺席（决策4：源文件缺失标记 / 无包内路径 / files 未收录）不落盘、不报错，
// 缺席计数由入库相位在挂载级统计。
func (ing *ingestor) extractFiles(ctx context.Context, manifest *export.Manifest, toExtract []*export.WorkRecord, fileSource FileSource) (*filePhaseResult, error) {
	result := &filePhaseResult{storeRemap: make(map[int64]int64)}

	// 结构校验 + 收集待提取的文件条目（按挂载遍历序去重保序）
	filesByStoreID := make(map[int64]*export.FileEntry, len(manifest.Files))
	for i := range manifest.Files {
		filesByStoreID[manifest.Files[i].StoreID] = &manifest.Files[i]
	}
	seen := make(map[int64]struct{})
	var needed []*export.FileEntry
	for _, w := range toExtract {
		for _, res := range w.Resources {
			if err := entity.ValidateResourceType(res.ResourceType); err != nil {
				return result, fmt.Errorf("作品 %d 资源类型不合法: %w", w.ID, err)
			}
			for _, mount := range res.Stores {
				if err := entity.ValidateStoreType(mount.StoreType); err != nil {
					return result, fmt.Errorf("作品 %d 资源 store 角色不合法: %w", w.ID, err)
				}
				if mount.Generation != entity.GenerationDownloaded && mount.Generation != entity.GenerationDerived {
					return result, fmt.Errorf("作品 %d 资源 store 生成方式不合法: %q", w.ID, mount.Generation)
				}
				if _, done := seen[mount.StoreID]; done {
					continue
				}
				seen[mount.StoreID] = struct{}{}
				if entry, ok := filesByStoreID[mount.StoreID]; ok {
					needed = append(needed, entry)
				}
			}
		}
	}
	if len(needed) == 0 {
		return result, nil
	}
	if fileSource == nil {
		return result, ErrNoFileSource
	}

	// 逐条目落盘：路径冲突消解 → 流式落盘（内容哈希随流计算）→ sha256 校验
	claimed := make(map[string]struct{}, len(needed))
	for _, entry := range needed {
		if entry.Missing || entry.Path == "" || entry.StorePath == "" {
			continue // 挂载缺席，入库相位按挂载级统计
		}
		if err := validateRelPath(entry.StorePath); err != nil {
			return result, err
		}
		relPath, err := ing.claimPath(ctx, entry.StorePath, claimed)
		if err != nil {
			return result, err
		}
		src, err := fileSource(entry.Path)
		if err != nil {
			return result, fmt.Errorf("%w：%s", ErrPackageFileMissing, entry.Path)
		}
		hasher := sha256.New()
		newID, storeErr := ing.fileStore.Store(ctx, relPath, path.Base(entry.StorePath), io.TeeReader(src, hasher))
		closeErr := src.Close()
		if storeErr != nil {
			return result, fmt.Errorf("落盘文件失败 %s: %w", entry.StorePath, storeErr)
		}
		result.createdStores = append(result.createdStores, newID)
		if closeErr != nil {
			return result, fmt.Errorf("关闭包内文件失败 %s: %w", entry.Path, closeErr)
		}
		if entry.Sha256 != "" && hex.EncodeToString(hasher.Sum(nil)) != entry.Sha256 {
			return result, fmt.Errorf("%w：%s", ErrChecksumMismatch, entry.Path)
		}
		result.storeRemap[entry.StoreID] = newID
		result.extractedFiles++
	}
	return result, nil
}

// validateRelPath 校验导出文件的目标落盘路径（relPath 域基准：正斜杠、无穿越段），
// 并要求命中 storeRegistry 已注册子目录白名单。
func validateRelPath(relPath string) error {
	unsafe := relPath == "" ||
		path.IsAbs(relPath) ||
		strings.ContainsRune(relPath, '\\') ||
		strings.Contains(relPath, "../") ||
		relPath != path.Clean(relPath)
	if unsafe {
		return fmt.Errorf("%w：%s", ErrUnsafeStorePath, relPath)
	}
	if err := storeRegistry.ValidatePath(relPath); err != nil {
		return fmt.Errorf("%w：%v", ErrUnsafeStorePath, err)
	}
	return nil
}

// claimPath 为落盘目标路径消解冲突：与库内既有活行 persistent_store 记录及本次导入已占用
// 路径均不重叠时采用原路径；冲突时保留目录与扩展名、文件名追加 _import<n> 序号派生变体
// （避免 persistentStore.Store 的同路径覆盖语义改写既有作品的文件与记录）。
func (ing *ingestor) claimPath(ctx context.Context, desired string, claimed map[string]struct{}) (string, error) {
	ext := path.Ext(desired)
	stem := strings.TrimSuffix(path.Base(desired), ext)
	dir := path.Dir(desired)
	for i := 0; ; i++ {
		candidate := desired
		if i > 0 {
			candidate = path.Join(dir, fmt.Sprintf("%s_import%d%s", stem, i, ext))
		}
		if _, taken := claimed[candidate]; taken {
			continue
		}
		rec, err := ing.fileStore.GetByFilePath(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("查询落盘路径占用失败 %s: %w", candidate, err)
		}
		if rec == nil {
			claimed[candidate] = struct{}{}
			return candidate, nil
		}
	}
}

// cleanupCreatedStores 导入失败补偿清理：物理删除本次已创建的 persistent_store 记录与文件
// （不产生备份——半成品导入不留残余）。失败仅记日志，补偿自身不阻断错误上报。
func (ing *ingestor) cleanupCreatedStores(ctx context.Context, storeIds []int64) {
	for _, id := range storeIds {
		if _, err := ing.fileStore.HardDelete(ctx, id, false); err != nil {
			logger.Log.Warnf("导入失败补偿清理文件记录失败 storeId=%d: %v", id, err)
		}
	}
}

// ===== 相位三：单事务入库 =====

// ingestEntities 事务内重建全部主数据与关联。顺序满足外键依赖：
// 站点 → 本地标签（层级按轮次）→ 本地作者 → 站点标签/站点作者 → 作品 → 资源 →
// resource_store 挂载 → 作品集（封面引用作品，故在作品后）→ 作品集父边 → 作品关联。
// 替换分支（toReplace）叠加在作品相位：元数据覆盖 → 空壳净化 → manifest 资源/挂载新建到
// 已有作品 → 关联合并挂载。
func (ing *ingestor) ingestEntities(
	ctx context.Context,
	manifest *export.Manifest,
	toCreate []*export.WorkRecord,
	toReplace []*export.WorkRecord,
	existingWorks map[int64]int64,
	opts *IngestOptions,
	storeRemap map[int64]int64,
) (*ImportResult, error) {
	result := &ImportResult{}

	siteRemap, createdSites, err := ing.ensureSites(ctx, manifest.Sites)
	if err != nil {
		return nil, err
	}
	result.CreatedSites = createdSites

	localTagRemap, createdLocalTags, err := ing.ensureLocalTags(ctx, manifest.LocalTags)
	if err != nil {
		return nil, err
	}
	result.CreatedLocalTags = createdLocalTags

	localAuthorRemap, createdLocalAuthors, err := ing.ensureLocalAuthors(ctx, manifest.LocalAuthors)
	if err != nil {
		return nil, err
	}
	result.CreatedLocalAuthors = createdLocalAuthors

	siteTagRemap, createdSiteTags, err := ing.ensureSiteTags(ctx, manifest.SiteTags, siteRemap, localTagRemap)
	if err != nil {
		return nil, err
	}
	result.CreatedSiteTags = createdSiteTags

	siteAuthorRemap, createdSiteAuthors, err := ing.ensureSiteAuthors(ctx, manifest.SiteAuthors, siteRemap, localAuthorRemap)
	if err != nil {
		return nil, err
	}
	result.CreatedSiteAuthors = createdSiteAuthors

	// 作品：查重命中并入重映射（作品集封面解析需要），待建作品批量落库
	workRemap := make(map[int64]int64, len(existingWorks)+len(toCreate))
	for manifestID, localID := range existingWorks {
		workRemap[manifestID] = localID
	}
	if len(toCreate) > 0 {
		workRows := make([]*entity.Work, 0, len(toCreate))
		for _, w := range toCreate {
			workRows = append(workRows, buildWorkEntity(w, siteRemap, localAuthorRemap))
		}
		if err := ing.repo.CreateWorks(ctx, workRows); err != nil {
			return nil, fmt.Errorf("新建作品失败: %w", err)
		}
		for i, w := range toCreate {
			workRemap[w.ID] = workRows[i].GetID()
		}
	}
	result.CreatedWorks = int64(len(toCreate))
	result.SkippedWorks = int64(len(manifest.Works)) - int64(len(toCreate)) - int64(len(toReplace))
	result.ReplacedWorks = int64(len(toReplace))
	result.ReplacedConfirmed, result.ReplacedAuto = countReplaceByKind(toReplace, opts)

	// 替换作品的元数据覆盖：work 行字段按 manifest 覆盖更新（站点侧名称等），workID 不变
	for _, w := range toReplace {
		localWorkID := existingWorks[w.ID]
		if err := ing.repo.UpdateWork(ctx, buildReplaceWorkUpdate(w, siteRemap, localAuthorRemap, localWorkID)); err != nil {
			return nil, fmt.Errorf("更新替换作品元数据失败: %w", err)
		}
	}

	// 替换作品的空壳净化：被软删光活行 store 的已有 resource 成空壳（无活行内容），
	// 物理删其残留关联行与资源行——软删行关联保留是替换链语义（供失败回滚复活挂载回位），
	// 但空壳资源行已无活行内容，其残留关联指向软删 store 行，按 DEAD 行处理规则净化
	for _, w := range toReplace {
		localWorkID := existingWorks[w.ID]
		emptyShellIDs, err := ing.repo.ListEmptyShellResourceIdsByWorkId(ctx, localWorkID)
		if err != nil {
			return nil, fmt.Errorf("查询替换作品空壳资源失败: %w", err)
		}
		if len(emptyShellIDs) == 0 {
			continue
		}
		if err := ing.repo.DeleteResourceStoresByResourceIds(ctx, emptyShellIDs); err != nil {
			return nil, fmt.Errorf("清理替换作品空壳资源挂载失败: %w", err)
		}
		if err := ing.repo.DeleteResourcesByResourceIds(ctx, emptyShellIDs); err != nil {
			return nil, fmt.Errorf("清理替换作品空壳资源失败: %w", err)
		}
	}

	// 资源（源库任务不在本库存在，task_id 溯源链断开置 NULL）：
	// 新建作品资源全新建；替换作品 manifest 的资源以新资源行挂到已有作品
	allWorks := make([]*export.WorkRecord, 0, len(toCreate)+len(toReplace))
	allWorks = append(allWorks, toCreate...)
	allWorks = append(allWorks, toReplace...)
	resourceRemap := make(map[int64]int64)
	var resourceRows []*entity.Resource
	var resourceRecords []*export.ResourceRecord // 与 resourceRows 平行：落库后回填重映射
	for _, w := range allWorks {
		for i := range w.Resources {
			res := &w.Resources[i]
			resourceRecords = append(resourceRecords, res)
			resourceRows = append(resourceRows, buildResourceEntity(res, workRemap[w.ID]))
		}
	}
	if err := ing.repo.CreateResources(ctx, resourceRows); err != nil {
		return nil, fmt.Errorf("新建资源失败: %w", err)
	}
	for i := range resourceRows {
		resourceRemap[resourceRecords[i].ID] = resourceRows[i].GetID()
	}

	// resource_store 挂载（文件引用按 storeRemap 重映射；缺席挂载跳过并计数）
	var mountRows []*entity.ResourceStore
	for _, w := range allWorks {
		for i := range w.Resources {
			res := &w.Resources[i]
			for _, mount := range res.Stores {
				newStoreID, ok := storeRemap[mount.StoreID]
				if !ok {
					result.AbsentStores++ // 决策4：挂载缺席（源缺失/无包内路径/files 未收录）
					continue
				}
				mountRows = append(mountRows, buildResourceStoreEntity(mount, resourceRemap[res.ID], newStoreID))
			}
		}
	}
	if err := ing.repo.CreateResourceStores(ctx, mountRows); err != nil {
		return nil, fmt.Errorf("新建资源挂载失败: %w", err)
	}

	// 作品集：查重（site_id+site_work_set_id，与作品同型）+ 新建（封面引用已就位的作品）+ 父集边
	workSetRemap, createdSets, skippedSets, err := ing.ensureWorkSets(ctx, manifest.WorkSets, siteRemap, workRemap)
	if err != nil {
		return nil, err
	}
	result.CreatedWorkSets = createdSets
	result.SkippedWorkSets = skippedSets

	// 作品关联：新建作品全新建；替换作品与本地已有关联去重合并挂载（本地已有关联不删）
	replaceLocalIDs := make(map[int64]struct{}, len(toReplace))
	for _, w := range toReplace {
		replaceLocalIDs[existingWorks[w.ID]] = struct{}{}
	}
	if err := ing.ensureWorkLinks(ctx, toCreate, toReplace, replaceLocalIDs, workRemap, localTagRemap, siteTagRemap, localAuthorRemap, siteAuthorRemap, workSetRemap); err != nil {
		return nil, err
	}
	return result, nil
}

// countReplaceByKind 按替换来源分类拆分计数（确认替换 / 零交集自动增补）。
// opts 为空或作品不在确认集时归入自动增补（替换全集 = 确认集 ∪ 自动集，重叠按确认计）。
func countReplaceByKind(toReplace []*export.WorkRecord, opts *IngestOptions) (confirmed, auto int64) {
	for _, w := range toReplace {
		if opts != nil {
			if _, ok := opts.ReplaceWorks[w.ID]; ok {
				confirmed++
				continue
			}
		}
		auto++
	}
	return confirmed, auto
}

// ===== 主数据 find-or-create =====

// ensureSites 站点 find-or-create（按键匹配，键在本库已存在即复用）。
// 新建行的键取 manifest 记录键（经入口 identity 校验已注册），名称与主页取注册表权威值——
// manifest 携带的站点展示信息不落库（站点展示信息以注册表为准）。
// 返回导出库站点 ID → 本库站点 ID 的全量重映射与新建数。
func (ing *ingestor) ensureSites(ctx context.Context, records []export.SiteRecord) (map[int64]int64, int64, error) {
	keys := make([]string, 0, len(records))
	for _, r := range records {
		keys = append(keys, r.SiteKey)
	}
	rows, err := ing.dupRepo.ListSitesByKeys(ctx, util.UniqueString(keys))
	if err != nil {
		return nil, 0, fmt.Errorf("查询既有站点失败: %w", err)
	}
	remap := make(map[int64]int64, len(records))
	keyToLocalID := make(map[string]int64, len(rows))
	for _, row := range rows {
		keyToLocalID[row.SiteKey] = row.GetID()
	}

	var creates []*entity.Site
	var createRecordIDs []int64
	sessionIdx := make(map[string]int) // 会话内已建行的键 → creates 下标（同批/跨批同键折叠）
	type dupRef struct {
		recordID  int64
		createIdx int
	}
	var dups []dupRef
	for _, r := range records {
		if localID, ok := keyToLocalID[r.SiteKey]; ok {
			remap[r.ID] = localID
			continue
		}
		if idx, ok := sessionIdx[r.SiteKey]; ok {
			dups = append(dups, dupRef{r.ID, idx})
			continue
		}
		entry, _ := identity.Lookup(r.SiteKey)
		e := entity.NewSite()
		e.SiteKey = entry.Key
		e.SiteName = sql.NullString{String: entry.Name, Valid: true}
		if entry.Homepage != "" {
			e.Homepage = sql.NullString{String: entry.Homepage, Valid: true}
		}
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		creates = append(creates, e)
		createRecordIDs = append(createRecordIDs, r.ID)
		sessionIdx[r.SiteKey] = len(creates) - 1
	}
	if err := ing.repo.CreateSites(ctx, creates); err != nil {
		return nil, 0, fmt.Errorf("新建站点失败: %w", err)
	}
	for i, e := range creates {
		remap[createRecordIDs[i]] = e.GetID()
	}
	for _, d := range dups {
		remap[d.recordID] = creates[d.createIdx].GetID()
	}
	return remap, int64(len(creates)), nil
}

// ensureLocalTags 本地标签 find-or-create（决策15：按名称匹配，同名复用——源库允许重名行，
// 导入坍缩为同一本库行）。层级（base_local_tag_id）按轮次创建：父引用已解析（既有行或已建行）
// 即建；父引用悬空（父不在 manifest 内）挂 NULL 降级入库；全轮无进展（环状引用的异常产物）
// 时剩余标签全部挂 NULL 降级。返回导出库标签 ID → 本库 ID 重映射与新建数。
func (ing *ingestor) ensureLocalTags(ctx context.Context, records []export.TagRecord) (map[int64]int64, int64, error) {
	names := make([]string, 0, len(records))
	for _, r := range records {
		if r.Name != nil && *r.Name != "" {
			names = append(names, *r.Name)
		}
	}
	rows, err := ing.repo.ListLocalTagsByNames(ctx, util.UniqueString(names))
	if err != nil {
		return nil, 0, fmt.Errorf("查询既有本地标签失败: %w", err)
	}
	remap := make(map[int64]int64, len(records))
	nameToLocalID := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.LocalTagName.Valid {
			nameToLocalID[row.LocalTagName.String] = row.GetID()
		}
	}
	inManifest := make(map[int64]struct{}, len(records))
	for _, r := range records {
		inManifest[r.ID] = struct{}{}
	}

	var pending []*export.TagRecord
	for i := range records {
		r := &records[i]
		if r.Name != nil && *r.Name != "" {
			if localID, ok := nameToLocalID[*r.Name]; ok {
				remap[r.ID] = localID
				continue
			}
		}
		pending = append(pending, r)
	}

	// tagBatch 单轮新建批：行 + 对应记录 + 同批重名折叠的引用回填
	type tagBatch struct {
		rows    []*entity.LocalTag
		recIDs  []int64
		nameIdx map[string]int // 名 → 本批首行下标
	}
	// 会话内已建行的名 → 本库 ID（跨轮次重名复用）
	sessionCreated := make(map[string]int64)
	var created int64

	buildTag := func(r *export.TagRecord, base sql.NullInt64) *entity.LocalTag {
		e := entity.NewLocalTag()
		e.LocalTagName = nullStringFromPtr(r.Name, true)
		e.BaseLocalTagID = base
		e.Description = nullStringFromPtr(r.Description, false)
		e.LastUse = nullInt64FromPtr(r.LastUse)
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		return e
	}

	for len(pending) > 0 {
		batch := &tagBatch{nameIdx: make(map[string]int)}
		var deferred []*export.TagRecord
		type batchDup struct {
			recordID int64
			name     string
		}
		var batchDups []batchDup
		// 会话级重名先折（含此前轮次与上一轮 flush 后的行）
		rest := pending[:0:0]
		for _, r := range pending {
			if r.Name != nil && *r.Name != "" {
				if localID, ok := sessionCreated[*r.Name]; ok {
					remap[r.ID] = localID
					continue
				}
			}
			rest = append(rest, r)
		}
		for _, r := range rest {
			// 同轮批内重名折叠
			if r.Name != nil && *r.Name != "" {
				if _, inBatch := batch.nameIdx[*r.Name]; inBatch {
					batchDups = append(batchDups, batchDup{r.ID, *r.Name})
					continue
				}
			}
			if r.BaseLocalTagID == nil {
				batch.rows = append(batch.rows, buildTag(r, sql.NullInt64{}))
				batch.recIDs = append(batch.recIDs, r.ID)
			} else if parentLocal, ok := remap[*r.BaseLocalTagID]; ok {
				batch.rows = append(batch.rows, buildTag(r, sql.NullInt64{Int64: parentLocal, Valid: true}))
				batch.recIDs = append(batch.recIDs, r.ID)
			} else if _, inSet := inManifest[*r.BaseLocalTagID]; !inSet {
				batch.rows = append(batch.rows, buildTag(r, sql.NullInt64{})) // 父引用悬空：挂 NULL 降级
				batch.recIDs = append(batch.recIDs, r.ID)
			} else {
				deferred = append(deferred, r) // 父在本 manifest 内但未建，等下一轮
			}
			if r.Name != nil && *r.Name != "" && len(batch.rows) > 0 && batch.recIDs[len(batch.recIDs)-1] == r.ID {
				batch.nameIdx[*r.Name] = len(batch.rows) - 1
			}
		}

		if len(batch.rows) > 0 {
			if err := ing.repo.CreateLocalTags(ctx, batch.rows); err != nil {
				return nil, 0, fmt.Errorf("新建本地标签失败: %w", err)
			}
			for i, e := range batch.rows {
				remap[batch.recIDs[i]] = e.GetID()
				if e.LocalTagName.Valid {
					sessionCreated[e.LocalTagName.String] = e.GetID()
				}
			}
			created += int64(len(batch.rows))
			for _, d := range batchDups {
				remap[d.recordID] = batch.rows[batch.nameIdx[d.name]].GetID()
			}
		}
		if len(batch.rows) == 0 && len(deferred) > 0 {
			// 全轮无进展：剩余为环状引用，全部挂 NULL 降级入库
			for _, r := range deferred {
				e := buildTag(r, sql.NullInt64{})
				batch.rows = append(batch.rows, e)
				batch.recIDs = append(batch.recIDs, r.ID)
			}
			if err := ing.repo.CreateLocalTags(ctx, batch.rows); err != nil {
				return nil, 0, fmt.Errorf("新建本地标签失败: %w", err)
			}
			for i, e := range batch.rows {
				remap[batch.recIDs[i]] = e.GetID()
			}
			created += int64(len(batch.rows))
			logger.Log.Warnf("导入本地标签存在环状父引用，%d 条按根标签降级入库", len(deferred))
			break
		}
		pending = deferred
	}
	return remap, created, nil
}

// ensureLocalAuthors 本地作者 find-or-create（决策15：按名称匹配，同名复用；源库重名行坍缩）。
func (ing *ingestor) ensureLocalAuthors(ctx context.Context, records []export.AuthorRecord) (map[int64]int64, int64, error) {
	names := make([]string, 0, len(records))
	for _, r := range records {
		if r.Name != nil && *r.Name != "" {
			names = append(names, *r.Name)
		}
	}
	rows, err := ing.repo.ListLocalAuthorsByNames(ctx, util.UniqueString(names))
	if err != nil {
		return nil, 0, fmt.Errorf("查询既有本地作者失败: %w", err)
	}
	remap := make(map[int64]int64, len(records))
	nameToLocalID := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.AuthorName.Valid {
			nameToLocalID[row.AuthorName.String] = row.GetID()
		}
	}

	var creates []*entity.LocalAuthor
	var createRecordIDs []int64
	sessionIdx := make(map[string]int)
	type dupRef struct {
		recordID  int64
		createIdx int
	}
	var dups []dupRef
	for _, r := range records {
		if r.Name != nil && *r.Name != "" {
			if localID, ok := nameToLocalID[*r.Name]; ok {
				remap[r.ID] = localID
				continue
			}
			if idx, ok := sessionIdx[*r.Name]; ok {
				dups = append(dups, dupRef{r.ID, idx})
				continue
			}
		}
		e := entity.NewLocalAuthor()
		e.AuthorName = nullStringFromPtr(r.Name, true)
		e.Introduce = nullStringFromPtr(r.Introduce, false)
		e.LastUse = nullInt64FromPtr(r.LastUse)
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		creates = append(creates, e)
		createRecordIDs = append(createRecordIDs, r.ID)
		if r.Name != nil && *r.Name != "" {
			sessionIdx[*r.Name] = len(creates) - 1
		}
	}
	if err := ing.repo.CreateLocalAuthors(ctx, creates); err != nil {
		return nil, 0, fmt.Errorf("新建本地作者失败: %w", err)
	}
	for i, e := range creates {
		remap[createRecordIDs[i]] = e.GetID()
	}
	for _, d := range dups {
		remap[d.recordID] = creates[d.createIdx].GetID()
	}
	return remap, int64(len(creates)), nil
}

// siteTagKey 站点标签身份键（本库站点 ID + site_tag_id）。
type siteTagKey struct {
	siteID    int64
	siteTagID string
}

// siteAuthorKey 站点作者身份键（本库站点 ID + site_author_id）。
type siteAuthorKey struct {
	siteID       int64
	siteAuthorID string
}

// ensureSiteTags 站点标签 find-or-create：身份键 =（本库站点 ID + site_tag_id），与作品查重
// 同型（对齐 work 模块 upsertSiteTags 的复合键口径）。名称不参与匹配——站点标签有客观跨库
// 身份，重名歧义按决策15归 todo#10 事后合并。site→local 桥接与 namespace 按行保真。
// 站点缺引或站点不在 manifest 站点闭包内的记录无身份键，恒新建。
func (ing *ingestor) ensureSiteTags(ctx context.Context, records []export.TagRecord, siteRemap map[int64]int64, localTagRemap map[int64]int64) (map[int64]int64, int64, error) {
	found := make(map[siteTagKey]int64)
	bySite := make(map[int64][]string)
	for _, r := range records {
		if r.SiteID == nil || r.SiteTagID == nil {
			continue
		}
		localSiteID, ok := siteRemap[*r.SiteID]
		if !ok {
			continue
		}
		bySite[localSiteID] = append(bySite[localSiteID], *r.SiteTagID)
	}
	for localSiteID, ids := range bySite {
		rows, err := ing.repo.ListSiteTagsBySiteAndTagIDs(ctx, localSiteID, util.UniqueString(ids))
		if err != nil {
			return nil, 0, fmt.Errorf("查询既有站点标签失败: %w", err)
		}
		for _, row := range rows {
			if row.SiteID.Valid && row.SiteTagID.Valid {
				found[siteTagKey{row.SiteID.Int64, row.SiteTagID.String}] = row.GetID()
			}
		}
	}

	remap := make(map[int64]int64, len(records))
	var creates []*entity.SiteTag
	var createRecordIDs []int64
	sessionIdx := make(map[siteTagKey]int)
	type dupRef struct {
		recordID  int64
		createIdx int
	}
	var dups []dupRef
	for _, r := range records {
		var key siteTagKey
		keyable := false
		if r.SiteID != nil && r.SiteTagID != nil {
			if localSiteID, ok := siteRemap[*r.SiteID]; ok {
				key = siteTagKey{localSiteID, *r.SiteTagID}
				keyable = true
			}
		}
		if keyable {
			if localID, ok := found[key]; ok {
				remap[r.ID] = localID
				continue
			}
			if idx, ok := sessionIdx[key]; ok {
				dups = append(dups, dupRef{r.ID, idx})
				continue
			}
		}
		e := entity.NewSiteTag()
		if keyable {
			e.SiteID = sql.NullInt64{Int64: key.siteID, Valid: true}
			e.SiteTagID = nullStringFromPtr(r.SiteTagID, false)
		}
		e.SiteTagName = nullStringFromPtr(r.Name, false)
		e.BaseSiteTagID = nullStringFromPtr(r.BaseSiteTagID, false)
		e.Namespace = nullStringFromPtr(r.Namespace, true) // 落库守卫：空串按 NULL
		e.LocalTagID = remappedRef(r.LocalTagID, localTagRemap)
		e.Description = nullStringFromPtr(r.Description, false)
		e.LastUse = nullInt64FromPtr(r.LastUse)
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		creates = append(creates, e)
		createRecordIDs = append(createRecordIDs, r.ID)
		if keyable {
			sessionIdx[key] = len(creates) - 1
		}
	}
	if err := ing.repo.CreateSiteTags(ctx, creates); err != nil {
		return nil, 0, fmt.Errorf("新建站点标签失败: %w", err)
	}
	for i, e := range creates {
		remap[createRecordIDs[i]] = e.GetID()
	}
	for _, d := range dups {
		remap[d.recordID] = creates[d.createIdx].GetID()
	}
	return remap, int64(len(creates)), nil
}

// ensureSiteAuthors 站点作者 find-or-create：身份键 =（本库站点 ID + site_author_id），
// 与站点标签同型。site→local 桥接按行保真。
func (ing *ingestor) ensureSiteAuthors(ctx context.Context, records []export.AuthorRecord, siteRemap map[int64]int64, localAuthorRemap map[int64]int64) (map[int64]int64, int64, error) {
	found := make(map[siteAuthorKey]int64)
	bySite := make(map[int64][]string)
	for _, r := range records {
		if r.SiteID == nil || r.SiteAuthorID == nil {
			continue
		}
		localSiteID, ok := siteRemap[*r.SiteID]
		if !ok {
			continue
		}
		bySite[localSiteID] = append(bySite[localSiteID], *r.SiteAuthorID)
	}
	for localSiteID, ids := range bySite {
		rows, err := ing.repo.ListSiteAuthorsBySiteAndAuthorIDs(ctx, localSiteID, util.UniqueString(ids))
		if err != nil {
			return nil, 0, fmt.Errorf("查询既有站点作者失败: %w", err)
		}
		for _, row := range rows {
			if row.SiteID.Valid && row.SiteAuthorID.Valid {
				found[siteAuthorKey{row.SiteID.Int64, row.SiteAuthorID.String}] = row.GetID()
			}
		}
	}

	remap := make(map[int64]int64, len(records))
	var creates []*entity.SiteAuthor
	var createRecordIDs []int64
	sessionIdx := make(map[siteAuthorKey]int)
	type dupRef struct {
		recordID  int64
		createIdx int
	}
	var dups []dupRef
	for _, r := range records {
		var key siteAuthorKey
		keyable := false
		if r.SiteID != nil && r.SiteAuthorID != nil {
			if localSiteID, ok := siteRemap[*r.SiteID]; ok {
				key = siteAuthorKey{localSiteID, *r.SiteAuthorID}
				keyable = true
			}
		}
		if keyable {
			if localID, ok := found[key]; ok {
				remap[r.ID] = localID
				continue
			}
			if idx, ok := sessionIdx[key]; ok {
				dups = append(dups, dupRef{r.ID, idx})
				continue
			}
		}
		e := entity.NewSiteAuthor()
		if keyable {
			e.SiteID = sql.NullInt64{Int64: key.siteID, Valid: true}
			e.SiteAuthorID = nullStringFromPtr(r.SiteAuthorID, false)
		}
		e.AuthorName = nullStringFromPtr(r.Name, false)
		e.FixedAuthorName = nullStringFromPtr(r.FixedAuthorName, false)
		e.SiteAuthorNameBefore = nullStringFromPtr(r.SiteAuthorNameBefore, false)
		e.Homepage = nullStringFromPtr(r.Homepage, false)
		e.Introduce = nullStringFromPtr(r.Introduce, false)
		e.LocalAuthorID = remappedRef(r.LocalAuthorID, localAuthorRemap)
		e.LastUse = nullInt64FromPtr(r.LastUse)
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		creates = append(creates, e)
		createRecordIDs = append(createRecordIDs, r.ID)
		if keyable {
			sessionIdx[key] = len(creates) - 1
		}
	}
	if err := ing.repo.CreateSiteAuthors(ctx, creates); err != nil {
		return nil, 0, fmt.Errorf("新建站点作者失败: %w", err)
	}
	for i, e := range creates {
		remap[createRecordIDs[i]] = e.GetID()
	}
	for _, d := range dups {
		remap[d.recordID] = creates[d.createIdx].GetID()
	}
	return remap, int64(len(creates)), nil
}

// workSetKey 作品集查重键（本库站点 ID + site_work_set_id）。
type workSetKey struct {
	siteID        int64
	siteWorkSetID string
}

// ensureWorkSets 作品集 find-or-create：查重键 =（本库站点 ID + site_work_set_id），与作品同型；
// 无站点身份的作品集无跨库稳定身份，恒新建（与无站点身份作品同一兜底口径）。
// 封面（cover_work_id 引用作品，作品相位已就位）在创建时直接重映射；
// 父集边（re_work_set_work_set，仅新建作品集挂载）在全部作品集落位后批量建——
// 查重命中的作品集整体跳过，其层级归本库既有状态管理。
func (ing *ingestor) ensureWorkSets(ctx context.Context, records []export.WorkSetRecord, siteRemap map[int64]int64, workRemap map[int64]int64) (map[int64]int64, int64, int64, error) {
	found := make(map[workSetKey]int64)
	bySite := make(map[int64][]string)
	for _, r := range records {
		if r.SiteID == nil || r.SiteWorkSetID == nil || *r.SiteWorkSetID == "" {
			continue
		}
		localSiteID, ok := siteRemap[*r.SiteID]
		if !ok {
			continue
		}
		bySite[localSiteID] = append(bySite[localSiteID], *r.SiteWorkSetID)
	}
	for localSiteID, ids := range bySite {
		rows, err := ing.repo.ListWorkSetsBySiteAndSetIDs(ctx, localSiteID, util.UniqueString(ids))
		if err != nil {
			return nil, 0, 0, fmt.Errorf("查询既有作品集失败: %w", err)
		}
		for _, row := range rows {
			if row.SiteID.Valid && row.SiteWorkSetID.Valid {
				found[workSetKey{row.SiteID.Int64, row.SiteWorkSetID.String}] = row.GetID()
			}
		}
	}

	remap := make(map[int64]int64, len(records))
	var creates []*entity.WorkSet
	var createRecords []*export.WorkSetRecord
	sessionIdx := make(map[workSetKey]int)
	type dupRef struct {
		recordID  int64
		createIdx int
	}
	var dups []dupRef
	for i := range records {
		r := &records[i]
		var key workSetKey
		keyable := false
		if r.SiteID != nil && r.SiteWorkSetID != nil && *r.SiteWorkSetID != "" {
			if localSiteID, ok := siteRemap[*r.SiteID]; ok {
				key = workSetKey{localSiteID, *r.SiteWorkSetID}
				keyable = true
			}
		}
		if keyable {
			if localID, ok := found[key]; ok {
				remap[r.ID] = localID
				continue
			}
			if idx, ok := sessionIdx[key]; ok {
				dups = append(dups, dupRef{r.ID, idx})
				continue
			}
		}
		e := entity.NewWorkSet()
		if keyable {
			e.SiteID = sql.NullInt64{Int64: key.siteID, Valid: true}
			e.SiteWorkSetID = nullStringFromPtr(r.SiteWorkSetID, false)
		}
		e.SiteWorkSetName = nullStringFromPtr(r.SiteWorkSetName, false)
		e.SiteAuthorID = nullStringFromPtr(r.SiteAuthorID, false)
		e.SiteWorkSetDescription = nullStringFromPtr(r.SiteWorkSetDescription, false)
		e.SiteUploadTime = nullInt64FromPtr(r.SiteUploadTime)
		e.SiteUpdateTime = nullInt64FromPtr(r.SiteUpdateTime)
		e.NickName = nullStringFromPtr(r.NickName, false)
		e.Description = nullStringFromPtr(r.Description, false)
		e.LastView = nullInt64FromPtr(r.LastView)
		e.CoverWorkID = remappedRef(r.CoverWorkID, workRemap)
		e.SetCreateTime(r.CreateTime)
		e.SetUpdateTime(r.UpdateTime)
		creates = append(creates, e)
		createRecords = append(createRecords, r)
		if keyable {
			sessionIdx[key] = len(creates) - 1
		}
	}
	if err := ing.repo.CreateWorkSets(ctx, creates); err != nil {
		return nil, 0, 0, fmt.Errorf("新建作品集失败: %w", err)
	}
	for i, e := range creates {
		remap[createRecords[i].ID] = e.GetID()
	}
	for _, d := range dups {
		remap[d.recordID] = creates[d.createIdx].GetID()
	}

	// 父集边：仅新建作品集挂载；父引用按重映射解析，不在 manifest 闭包内的父集跳过该边。
	// 重映射坍缩可能令多条边落到同一 (parent, child) 对，按唯一索引键批内折叠，
	// 保留 manifest 首条的排序属性。
	type edgeKey struct {
		parent int64
		child  int64
	}
	var edges []*entity.ReWorkSetWorkSet
	edgeSeen := make(map[edgeKey]struct{})
	now := util.GetCurrentTimestamp()
	for _, r := range createRecords {
		childLocal := remap[r.ID]
		for _, p := range r.Parents {
			parentLocal, ok := remap[p.ParentWorkSetID]
			if !ok {
				continue
			}
			key := edgeKey{parentLocal, childLocal}
			if _, dup := edgeSeen[key]; dup {
				continue
			}
			edgeSeen[key] = struct{}{}
			edge := entity.NewReWorkSetWorkSet()
			edge.ParentWorkSetID = sql.NullInt64{Int64: parentLocal, Valid: true}
			edge.ChildWorkSetID = sql.NullInt64{Int64: childLocal, Valid: true}
			edge.SortOrder = nullInt64FromPtr(p.SortOrder)
			edge.SiteSortOrder = nullInt64FromPtr(p.SiteSortOrder)
			edge.SetCreateTime(now)
			edge.SetUpdateTime(now)
			edges = append(edges, edge)
		}
	}
	if err := ing.repo.CreateReWorkSetWorkSets(ctx, edges); err != nil {
		return nil, 0, 0, fmt.Errorf("新建作品集层级失败: %w", err)
	}
	return remap, int64(len(creates)), int64(len(records)) - int64(len(creates)), nil
}

// ensureWorkLinks 为新建作品挂标签/作者/作品集关联、为替换作品做关联合并。find-or-create 的
// 同名坍缩可能令多条 manifest 关联落到同一本库行，按各关联表的唯一索引键（work+tag /
// work+author / work+workset）批内折叠，保留 manifest 首条的关联级属性（namespace/role/sort 等）。
// 替换作品（replaceLocalIDs）预读本库已有关联键进 seen 防撞唯一索引——本地已有关联不删、
// manifest 关联增量挂载。
func (ing *ingestor) ensureWorkLinks(
	ctx context.Context,
	toCreate []*export.WorkRecord,
	toReplace []*export.WorkRecord,
	replaceLocalIDs map[int64]struct{},
	workRemap, localTagRemap, siteTagRemap, localAuthorRemap, siteAuthorRemap, workSetRemap map[int64]int64,
) error {
	now := util.GetCurrentTimestamp()
	var tagRows []*entity.ReWorkTag
	tagSeen := make(map[string]struct{})
	var authorRows []*entity.ReWorkAuthor
	authorSeen := make(map[string]struct{})
	var setRows []*entity.ReWorkWorkSet
	setSeen := make(map[string]struct{})

	// 替换作品的关联合并去重：预读本库已有关联键，防新增关联撞唯一索引
	if len(replaceLocalIDs) > 0 {
		ids := make([]int64, 0, len(replaceLocalIDs))
		for id := range replaceLocalIDs {
			ids = append(ids, id)
		}
		if rows, err := ing.repo.ListReWorkTagsByWorkIds(ctx, ids); err != nil {
			return fmt.Errorf("查询替换作品已有标签关联失败: %w", err)
		} else {
			for _, r := range rows {
				if k := existingTagLinkKey(r); k != "" {
					tagSeen[k] = struct{}{}
				}
			}
		}
		if rows, err := ing.repo.ListReWorkAuthorsByWorkIds(ctx, ids); err != nil {
			return fmt.Errorf("查询替换作品已有作者关联失败: %w", err)
		} else {
			for _, r := range rows {
				if k := existingAuthorLinkKey(r); k != "" {
					authorSeen[k] = struct{}{}
				}
			}
		}
		if rows, err := ing.repo.ListReWorkWorkSetsByWorkIds(ctx, ids); err != nil {
			return fmt.Errorf("查询替换作品已有作品集成员关系失败: %w", err)
		} else {
			for _, r := range rows {
				if k := existingWorkSetLinkKey(r); k != "" {
					setSeen[k] = struct{}{}
				}
			}
		}
	}

	for _, w := range append(append([]*export.WorkRecord{}, toCreate...), toReplace...) {
		localWorkID := workRemap[w.ID]
		for _, link := range w.TagLinks {
			e := entity.NewReWorkTag()
			e.WorkID = sql.NullInt64{Int64: localWorkID, Valid: true}
			e.TagType = sql.NullInt64{Int64: int64(link.TagType), Valid: true}
			var seenKey string
			switch link.TagType {
			case constant.LOCAL:
				localTagID, ok := localTagRemap[link.TagID]
				if !ok {
					continue // 引用悬空（数据异常），跳过该关联
				}
				e.LocalTagID = sql.NullInt64{Int64: localTagID, Valid: true}
				seenKey = fmt.Sprintf("%d|%d|%d", localWorkID, constant.LOCAL, localTagID)
			case constant.SITE:
				localSiteTagID, ok := siteTagRemap[link.TagID]
				if !ok {
					continue
				}
				e.SiteTagID = sql.NullInt64{Int64: localSiteTagID, Valid: true}
				seenKey = fmt.Sprintf("%d|%d|%d", localWorkID, constant.SITE, localSiteTagID)
			default:
				continue // 未知关联类型（数据异常），跳过
			}
			if _, dup := tagSeen[seenKey]; dup {
				continue
			}
			tagSeen[seenKey] = struct{}{}
			e.Namespace = nullStringFromPtr(link.Namespace, true) // 落库守卫：空串按 NULL
			e.SetCreateTime(now)
			e.SetUpdateTime(now)
			tagRows = append(tagRows, e)
		}
		for _, link := range w.AuthorLinks {
			e := entity.NewReWorkAuthor()
			e.WorkID = sql.NullInt64{Int64: localWorkID, Valid: true}
			e.AuthorType = sql.NullInt64{Int64: int64(link.AuthorType), Valid: true}
			var seenKey string
			switch link.AuthorType {
			case constant.LOCAL:
				localAuthorID, ok := localAuthorRemap[link.AuthorID]
				if !ok {
					continue
				}
				e.LocalAuthorID = sql.NullInt64{Int64: localAuthorID, Valid: true}
				seenKey = fmt.Sprintf("%d|%d|%d", localWorkID, constant.LOCAL, localAuthorID)
			case constant.SITE:
				localSiteAuthorID, ok := siteAuthorRemap[link.AuthorID]
				if !ok {
					continue
				}
				e.SiteAuthorID = sql.NullInt64{Int64: localSiteAuthorID, Valid: true}
				seenKey = fmt.Sprintf("%d|%d|%d", localWorkID, constant.SITE, localSiteAuthorID)
			default:
				continue
			}
			if _, dup := authorSeen[seenKey]; dup {
				continue
			}
			authorSeen[seenKey] = struct{}{}
			e.RoleName = nullStringFromPtr(link.RoleName, false)
			e.SortOrder = nullInt64FromPtr(link.SortOrder)
			e.SetCreateTime(now)
			e.SetUpdateTime(now)
			authorRows = append(authorRows, e)
		}
		for _, link := range w.WorkSetLinks {
			localSetID, ok := workSetRemap[link.WorkSetID]
			if !ok {
				continue // 引用的作品集不在 manifest 闭包内（数据异常），跳过
			}
			seenKey := fmt.Sprintf("%d|%d", localWorkID, localSetID)
			if _, dup := setSeen[seenKey]; dup {
				continue
			}
			setSeen[seenKey] = struct{}{}
			e := entity.NewReWorkWorkSet()
			e.WorkID = sql.NullInt64{Int64: localWorkID, Valid: true}
			e.WorkSetID = sql.NullInt64{Int64: localSetID, Valid: true}
			e.SortOrder = nullInt64FromPtr(link.SortOrder)
			e.SiteSortOrder = nullInt64FromPtr(link.SiteSortOrder)
			e.SetCreateTime(now)
			e.SetUpdateTime(now)
			setRows = append(setRows, e)
		}
	}

	if err := ing.repo.CreateReWorkTags(ctx, tagRows); err != nil {
		return fmt.Errorf("新建作品标签关联失败: %w", err)
	}
	if err := ing.repo.CreateReWorkAuthors(ctx, authorRows); err != nil {
		return fmt.Errorf("新建作品作者关联失败: %w", err)
	}
	if err := ing.repo.CreateReWorkWorkSets(ctx, setRows); err != nil {
		return fmt.Errorf("新建作品集成员关系失败: %w", err)
	}
	return nil
}

// existingTagLinkKey 本库已有标签关联的去重键（work_id + 标签类型 + 本库标签 ID），
// 与 ensureWorkLinks 新增关联的 seenKey 同型（按唯一索引键去重）。
func existingTagLinkKey(r *entity.ReWorkTag) string {
	if !r.WorkID.Valid {
		return ""
	}
	if r.LocalTagID.Valid {
		return fmt.Sprintf("%d|%d|%d", r.WorkID.Int64, constant.LOCAL, r.LocalTagID.Int64)
	}
	if r.SiteTagID.Valid {
		return fmt.Sprintf("%d|%d|%d", r.WorkID.Int64, constant.SITE, r.SiteTagID.Int64)
	}
	return ""
}

// existingAuthorLinkKey 本库已有作者关联的去重键（work_id + 作者类型 + 本库作者 ID）。
func existingAuthorLinkKey(r *entity.ReWorkAuthor) string {
	if !r.WorkID.Valid {
		return ""
	}
	if r.LocalAuthorID.Valid {
		return fmt.Sprintf("%d|%d|%d", r.WorkID.Int64, constant.LOCAL, r.LocalAuthorID.Int64)
	}
	if r.SiteAuthorID.Valid {
		return fmt.Sprintf("%d|%d|%d", r.WorkID.Int64, constant.SITE, r.SiteAuthorID.Int64)
	}
	return ""
}

// existingWorkSetLinkKey 本库已有作品集成员关系的去重键（work_id + 作品集 ID）。
func existingWorkSetLinkKey(r *entity.ReWorkWorkSet) string {
	if !r.WorkID.Valid || !r.WorkSetID.Valid {
		return ""
	}
	return fmt.Sprintf("%d|%d", r.WorkID.Int64, r.WorkSetID.Int64)
}

// ===== manifest 记录 → 实体转换 =====

// buildWorkEntity manifest 作品记录 → 作品实体（site/local_author 引用重映射，时间戳保真）。
func buildWorkEntity(r *export.WorkRecord, siteRemap, localAuthorRemap map[int64]int64) *entity.Work {
	w := entity.NewWork()
	if r.SiteID != nil {
		if localID, ok := siteRemap[*r.SiteID]; ok {
			w.SiteID = sql.NullInt64{Int64: localID, Valid: true}
		}
	}
	w.SiteWorkID = nullStringFromPtr(r.SiteWorkID, false)
	w.SiteWorkName = nullStringFromPtr(r.SiteWorkName, false)
	w.SiteAuthorID = nullStringFromPtr(r.SiteAuthorID, false)
	w.SiteWorkDescription = nullStringFromPtr(r.SiteWorkDescription, false)
	w.SiteUploadTime = nullInt64FromPtr(r.SiteUploadTime)
	w.SiteUpdateTime = nullInt64FromPtr(r.SiteUpdateTime)
	w.NickName = nullStringFromPtr(r.NickName, false)
	if r.LocalAuthorID != nil {
		if localID, ok := localAuthorRemap[*r.LocalAuthorID]; ok {
			w.LocalAuthorID = sql.NullInt64{Int64: localID, Valid: true}
		}
	}
	w.LastView = nullInt64FromPtr(r.LastView)
	w.SetCreateTime(r.CreateTime)
	w.SetUpdateTime(r.UpdateTime)
	return w
}

// buildReplaceWorkUpdate 替换作品的元数据覆盖更新实体：work 行字段按 manifest 覆盖
// （站点侧名称/描述/时间戳等），workID 不变。create_time 保留零值（本地入库时刻为权威，
// GORM Updates 跳过零值字段即不改写），update_time 按 manifest 源库更新时刻落。
func buildReplaceWorkUpdate(r *export.WorkRecord, siteRemap, localAuthorRemap map[int64]int64, localWorkID int64) *entity.Work {
	w := entity.NewWork()
	w.SetID(localWorkID)
	if r.SiteID != nil {
		if localID, ok := siteRemap[*r.SiteID]; ok {
			w.SiteID = sql.NullInt64{Int64: localID, Valid: true}
		}
	}
	w.SiteWorkID = nullStringFromPtr(r.SiteWorkID, false)
	w.SiteWorkName = nullStringFromPtr(r.SiteWorkName, false)
	w.SiteAuthorID = nullStringFromPtr(r.SiteAuthorID, false)
	w.SiteWorkDescription = nullStringFromPtr(r.SiteWorkDescription, false)
	w.SiteUploadTime = nullInt64FromPtr(r.SiteUploadTime)
	w.SiteUpdateTime = nullInt64FromPtr(r.SiteUpdateTime)
	w.NickName = nullStringFromPtr(r.NickName, false)
	if r.LocalAuthorID != nil {
		if localID, ok := localAuthorRemap[*r.LocalAuthorID]; ok {
			w.LocalAuthorID = sql.NullInt64{Int64: localID, Valid: true}
		}
	}
	w.LastView = nullInt64FromPtr(r.LastView)
	w.SetUpdateTime(r.UpdateTime)
	return w
}

// buildResourceEntity manifest 资源记录 → 资源实体（归属作品重映射；源库任务不在本库存在，
// task_id 置 NULL——导入资源非本库任务产出，溯源链断开）。
func buildResourceEntity(r *export.ResourceRecord, localWorkID int64) *entity.Resource {
	e := entity.NewResource()
	e.WorkID = localWorkID
	e.TaskID = sql.NullInt64{}
	e.SuggestName = nullStringFromPtr(r.SuggestName, false)
	e.ResourceComplete = nullInt64FromPtr(r.ResourceComplete)
	e.ResourceType = r.ResourceType
	e.SetCreateTime(r.CreateTime)
	e.SetUpdateTime(r.UpdateTime)
	return e
}

// buildResourceStoreEntity manifest store 挂载 → resource_store 实体（资源与文件引用重映射）。
// 挂载无独立时间戳（manifest 契约未携带），按导入时刻落。
func buildResourceStoreEntity(mount export.StoreMount, localResourceID, localStoreID int64) *entity.ResourceStore {
	e := entity.NewResourceStore()
	e.ResourceID = localResourceID
	e.StoreType = mount.StoreType
	e.Generation = mount.Generation
	e.StoreSeq = mount.StoreSeq
	e.StoreID = localStoreID
	now := util.GetCurrentTimestamp()
	e.SetCreateTime(now)
	e.SetUpdateTime(now)
	return e
}

// ===== 通用工具 =====

// nullStringFromPtr 指针 → sql.NullString；emptyAsInvalid=true 时空串按 NULL 落库
// （身份名空串无匹配意义，且 site_name 唯一索引下空串会互相冲突）。
func nullStringFromPtr(p *string, emptyAsInvalid bool) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	if *p == "" && emptyAsInvalid {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// nullInt64FromPtr 指针 → sql.NullInt64。
func nullInt64FromPtr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// remappedRef 导出库引用 → 本库引用重映射；引用未命中重映射时落 NULL（引用悬空降级）。
func remappedRef(ref *int64, remap map[int64]int64) sql.NullInt64 {
	if ref == nil {
		return sql.NullInt64{}
	}
	if localID, ok := remap[*ref]; ok {
		return sql.NullInt64{Int64: localID, Valid: true}
	}
	return sql.NullInt64{}
}
