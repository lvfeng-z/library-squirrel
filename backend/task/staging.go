package task

// 任务暂存目录基建：任务属主作用域的派生、创建与清理，暂存文件命名，归属判活谓词。
// 暂存模式：下载内容先写 {workDir}/staging/download/{taskID}/，全部轨道写满后由下载执行面在
// 提交点统一 rename 进 store/ 最终路径——暂存期内长下载全程零 DB 副作用。
// 任务暂存作用域分两个属主根（staging/download 与 staging/share-receive），作用域键恒为任务 ID，
// 同根承载两种目录内容形态：
//   - 插件下载任务（download 根）：role_seq 键命名的暂存文件（见 StagingFileName）；
//   - 收件任务（share-receive 根）：父任务目录含共享 manifest.json，子任务目录为按清单内路径
//     镜像命名的暂存文件——父/子任务各占一个作用域，互为平级不嵌套。
// 作用域创建经 staging 能力包原子入口（临时名目录写好自证描述后 rename 正式名），目录派生与
// 创建/清理在本文件收口为任务 ID 维度的薄封装；启动清扫与回收策略由 staging 包统一派发。
// 暂存总根不在 store/ 白名单子树（storeRegistry.RegisteredDirs）与 backup/ 域内，
// fsmonitor 对其文件操作零感知（事件全被白名单过滤，无需抑制登记）。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/library-squirrel/backend/staging"
)

// 任务属主作用域的内容形态标签：记录目录内文件的布局形态，创建方定义并自行消费
// （staging 包只存取不解释）。
const (
	// downloadScopeContentShape 下载任务作用域：role_seq 键命名的暂存文件平铺。
	downloadScopeContentShape = "role-seq-flat"
	// receiveScopeContentShape 收件任务作用域：父任务目录含共享 manifest.json，子任务目录为
	// 按清单内路径镜像命名的暂存文件。
	receiveScopeContentShape = "manifest-or-mirror"
)

// DownloadStagingPath 下载任务暂存目录派生单点：{workDir}/staging/download/{taskID}/。
// 返回值属 absPath 域（含 workDir 的绝对路径），仅供 os.* 文件系统调用点现场消费。
// workDir 空串=未配置，由调用侧既有 workdir 守卫链（settings.RefuseIfUnconfigured）拦截，本函数不做守卫。
func DownloadStagingPath(workDir string, taskID int64) string {
	return staging.ScopePath(workDir, staging.OwnerDownload, strconv.FormatInt(taskID, 10))
}

// ReceiveStagingPath 收件任务暂存目录派生单点：{workDir}/staging/share-receive/{taskID}/
// （父/子任务各占一个作用域）。absPath 域语义同 DownloadStagingPath。
func ReceiveStagingPath(workDir string, taskID int64) string {
	return staging.ScopePath(workDir, staging.OwnerShareReceive, strconv.FormatInt(taskID, 10))
}

// ReceiveManifestRelPath 收件共享清单的 workDir 相对路径（relPath 域正斜杠）：
// staging/share-receive/{父任务ID}/manifest.json。share_task 领域行的 manifest_path 列存此值，
// 子任务执行面按列值直读。
func ReceiveManifestRelPath(parentTaskID int64) string {
	return path.Join(staging.RootName, string(staging.OwnerShareReceive), strconv.FormatInt(parentTaskID, 10), "manifest.json")
}

// EnsureDownloadScope 确保下载任务暂存作用域存在：不存在则经 staging 原子入口创建（写自证
// 描述），已存在（暂停/崩溃后恢复的续传场景）直接复用。返回作用域目录绝对路径（absPath 域）。
func EnsureDownloadScope(ctx context.Context, workDir string, taskID int64) (string, error) {
	return ensureTaskScope(ctx, workDir, staging.OwnerDownload, taskID, downloadScopeContentShape)
}

// EnsureReceiveScope 确保收件任务暂存作用域存在（父/子任务同入口，键=任务 ID）。复用语义同
// EnsureDownloadScope。
func EnsureReceiveScope(ctx context.Context, workDir string, taskID int64) (string, error) {
	return ensureTaskScope(ctx, workDir, staging.OwnerShareReceive, taskID, receiveScopeContentShape)
}

// ensureTaskScope 任务属主作用域的确保语义：CreateScope 首建，ErrScopeExists（作用域已在的
// 恢复场景）复用既有目录；其余错误（workDir 未配置/键非法/IO 失败）原样上抛。
func ensureTaskScope(ctx context.Context, workDir string, owner staging.Owner, taskID int64, contentShape string) (string, error) {
	key := strconv.FormatInt(taskID, 10)
	dir, err := staging.CreateScope(ctx, workDir, owner, key, contentShape)
	if errors.Is(err, staging.ErrScopeExists) {
		return staging.ScopePath(workDir, owner, key), nil
	}
	return dir, err
}

// StagingFileName 暂存文件名：role_seq 派生键（seq 为同 role 内 0-based 序号 = store_seq，三位零填充），
// 保留扩展名供人工诊断。续传定位与崩溃清扫直接按文件名还原 (role, seq) 身份，不依赖元数据重解析；
// 最终文件名由下载执行面在执行前解析派生，与暂存名解耦。ext 无前导点时补点，空串则无扩展名段。
func StagingFileName(role string, storeSeq int, ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return fmt.Sprintf("%s_%03d%s", role, storeSeq, ext)
}

// CleanupStagingByTaskIds 按任务 ID 集合清理任务暂存作用域（下载与收件通用：任务删除链以被删
// 全量 ID〔含子任务〕调用，收件父作用域〔含共享 manifest〕与子任务作用域随任务消亡一并清理；
// work 删除链治理：作品资源即删，残留暂存会在任务恢复时误续传已删作品的下载产物）。被删集合
// 可混两类任务，逐 ID 对两个任务属主根各回收一次（目录不存在为容忍态）；任一删除失败即返回。
func CleanupStagingByTaskIds(workDir string, taskIds []int64) error {
	if workDir == "" || len(taskIds) == 0 {
		return nil
	}
	for _, id := range taskIds {
		if id <= 0 {
			continue
		}
		if err := os.RemoveAll(DownloadStagingPath(workDir, id)); err != nil {
			return err
		}
		if err := os.RemoveAll(ReceiveStagingPath(workDir, id)); err != nil {
			return err
		}
	}
	return nil
}

// TaskIdLister 任务行 ID 全量装载（TaskRepository 实现）：暂存归属判活谓词的批量数据源。
type TaskIdLister interface {
	ListAllIds(ctx context.Context) ([]int64, error)
}

// NewStagingOwnerAlive 构造任务暂存归属判活谓词（staging.ScopeOwnerAlive）：一次性装载 task 表
// 全量 ID 集合，逐作用域 O(1) 判定（不逐目录查询任务行）。任务属主根下作用域键恒为任务 ID，
// 非数字键判死；启动清扫先于任何能创建作用域的服务执行，装载时刻的 ID 快照即权威。
func NewStagingOwnerAlive(ctx context.Context, lister TaskIdLister) (staging.ScopeOwnerAlive, error) {
	ids, err := lister.ListAllIds(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return func(scopeKey string) bool {
		id, perr := strconv.ParseInt(scopeKey, 10, 64)
		if perr != nil {
			return false
		}
		_, ok := set[id]
		return ok
	}, nil
}
