package task

// 下载任务的暂存目录基建：目录派生、暂存文件命名与启动清扫。
// 暂存模式：下载内容先写 {workDir}/task-staging/{taskID}/，全部轨道写满后由下载执行面在
// 提交点统一 rename 进 store/ 最终路径——暂存期内长下载全程零 DB 副作用。
// task-staging/ 不在 store/ 白名单子树（storeRegistry.RegisteredDirs）与 backup/ 域内，
// fsmonitor 对其文件操作零感知（事件全被白名单过滤，无需抑制登记）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StagingRootName workDir 下的任务暂存目录名（任务行一个子目录）
const StagingRootName = "task-staging"

// StagingPath 任务暂存目录派生单点：{workDir}/task-staging/{taskID}/。
// 返回值属 absPath 域（含 workDir 的绝对路径），仅供 os.* 文件系统调用点现场消费。
// workDir 空串=未配置，由调用侧既有 workdir 守卫链（settings.RefuseIfUnconfigured）拦截，本函数不做守卫。
func StagingPath(workDir string, taskID int64) string {
	return filepath.Join(workDir, StagingRootName, strconv.FormatInt(taskID, 10))
}

// StagingFileName 暂存文件名：role_seq 派生键（seq 为同 role 内 0-based 序号 = store_seq，三位零填充），
// 保留扩展名供人工诊断。续传定位与崩溃清扫直接按文件名还原 (role, seq) 身份，不依赖元数据重解析；
// 最终文件名由下载执行面的规划表持有，与暂存名解耦。ext 无前导点时补点，空串则无扩展名段。
func StagingFileName(role string, storeSeq int, ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return fmt.Sprintf("%s_%03d%s", role, storeSeq, ext)
}

// CleanupStagingByTaskIds 按任务 ID 集合清理下载暂存目录（work 删除链治理：作品资源即删，
// 残留暂存会在任务恢复时误续传已删作品的下载产物）。目录不存在为容忍态；任一删除失败即返回
func CleanupStagingByTaskIds(workDir string, taskIds []int64) error {
	if workDir == "" || len(taskIds) == 0 {
		return nil
	}
	for _, id := range taskIds {
		if id <= 0 {
			continue
		}
		if err := os.RemoveAll(StagingPath(workDir, id)); err != nil {
			return err
		}
	}
	return nil
}

// CleanupOrphanStaging 启动清扫：回收任务行已不存在的下载暂存目录（任务删除后暂存随之失去归属；
// 成功任务的暂存已在提交点消费后清理，此处兜底崩溃残留与已删任务残留）。任务行仍在的暂存目录
// （暂停态待恢复与非暂停态均在内）保留给恢复判定，不误删。exists 由调用方提供任务行存在性查询。
func CleanupOrphanStaging(workDir string, exists func(id int64) bool) error {
	if workDir == "" {
		return nil
	}
	root := filepath.Join(workDir, StagingRootName)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		id, perr := strconv.ParseInt(ent.Name(), 10, 64)
		if perr != nil || id <= 0 {
			continue
		}
		if !exists(id) {
			if rerr := os.RemoveAll(filepath.Join(root, ent.Name())); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}
