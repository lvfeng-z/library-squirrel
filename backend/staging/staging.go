package staging

// 暂存目录作用域能力包：统一暂存总根 {workDir}/staging/ 下按属主分根（download/share-receive/
// import/merge/export），每个作用域目录内一份 scope.json 自证描述（稳定键/内容形态/创建时刻，
// 导出形态另记目标位置与临时文件名账本）。根注册表在本包集中登记「属主 → 根名 → 回收策略」，
// 消费方只提供归属谓词，不各自实现清扫。
// 暂存总根不在 store/ 白名单（storeRegistry.RegisteredDirs）与 backup/ 域内，fsmonitor 对其
// 文件操作零感知（事件全被白名单过滤，无需抑制登记）。
// 时序契约：清扫入口 SweepAtStartup 须在应用启动序列中、先于任何能创建作用域的服务启动处同步
// 调用（由 app.go 装配保证）——此后创建的作用域不进本次清扫，不存在在途作用域被误回收的
// 并发窗口。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// 错误定义。
var (
	// ErrWorkDirEmpty 工作目录未配置。空串参与路径拼接会被静默相对化为进程工作目录，
	// 请求期入口对该状态显式拒绝（与 settings.RefuseIfUnconfigured 同语义方向）。
	ErrWorkDirEmpty = errors.New("工作目录未配置，无法创建暂存作用域")
	// ErrOwnerNotRegistered 属主未在根注册表登记——作用域只能建在已登记属主的根下。
	ErrOwnerNotRegistered = errors.New("未登记的暂存属主")
	// ErrInvalidScopeKey 作用域键非法（合法字符集见 scope.go 的 scopeKeyPattern）。
	ErrInvalidScopeKey = errors.New("非法的作用域键")
	// ErrEmptyContentShape 内容形态标签为空——描述必须自证内容形态。
	ErrEmptyContentShape = errors.New("内容形态不能为空")
	// ErrEmptyExportTargetDir 导出账本缺少目标目录。
	ErrEmptyExportTargetDir = errors.New("导出账本缺少目标目录")
	// ErrInvalidTempFileName 导出账本临时文件名不是纯文件名（含路径分隔符或目录游标）。
	ErrInvalidTempFileName = errors.New("导出账本临时文件名非法")
	// ErrScopeExists 同键作用域目录已存在（残留目录须先经 RemoveScope 回收再重建）。
	ErrScopeExists = errors.New("作用域目录已存在")
	// ErrScopeDescMissing 作用域目录内无 scope.json。
	ErrScopeDescMissing = errors.New("作用域描述缺失")
	// ErrScopeDescCorrupt 作用域描述不可解析或字段校验不过。
	ErrScopeDescCorrupt = errors.New("作用域描述损坏")
)

// RootName workDir 下的暂存总根目录名，各属主根的下挂父目录。
const RootName = "staging"

// Owner 暂存属主类别：值即属主根目录名（staging/ 下一级），也是根注册表的登记键。
type Owner string

// 登记属主清单（与根注册表对应，新增属主须两处同步登记）。
const (
	// OwnerDownload 插件下载任务，作用域键=任务 ID。
	OwnerDownload Owner = "download"
	// OwnerShareReceive 分享收件任务：父/子任务各占一个作用域，键=任务 ID，父目录含共享 manifest。
	OwnerShareReceive Owner = "share-receive"
	// OwnerImport UI 回灌导入，作用域键=铸造稳定键，启动一律回收。
	OwnerImport Owner = "import"
	// OwnerMerge 合并产物，作用域键=铸造稳定键，启动一律回收。
	OwnerMerge Owner = "merge"
	// OwnerExport 导出：作用域目录内只放描述，物理临时文件留在最终产物的目标目录同级（跨卷
	// rename 不可绕），目标位置与临时文件名记入描述账本。
	OwnerExport Owner = "export"
)

// ScopePath 作用域目录绝对路径派生单点：{workDir}/staging/{owner}/{scopeKey}/。
// 返回值属 absPath 域（含 workDir 的绝对路径），仅供 os.* 文件系统调用点现场消费，禁止入库、
// 进事件或作比较键。workDir 空串=未配置，由调用侧既有 workdir 守卫链（settings.RefuseIfUnconfigured）
// 拦截，本函数不做守卫。
func ScopePath(workDir string, owner Owner, scopeKey string) string {
	return filepath.Join(workDir, RootName, string(owner), scopeKey)
}

// ownerRootPath 属主根目录绝对路径（absPath 域，仅供 os.* 调用点现场消费）。
func ownerRootPath(workDir string, owner Owner) string {
	return filepath.Join(workDir, RootName, string(owner))
}

// RemoveScope 回收单个作用域目录（任务完成/删除链调用）；目录不存在为容忍态。
func RemoveScope(ctx context.Context, workDir string, owner Owner, scopeKey string) error {
	if workDir == "" {
		return nil
	}
	return os.RemoveAll(ScopePath(workDir, owner, scopeKey))
}
