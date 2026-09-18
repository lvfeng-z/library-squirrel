package staging

// 根注册表：暂存根的唯一权威清单。新增属主根的唯一登记点是本表——包外不得另建暂存根；
// 统一清扫派发以本表为准，暂存总根下表外条目按无主数据回收。

import "fmt"

// reclaimPolicy 根回收策略：清扫时该属主根下作用域的去留判定方式。
type reclaimPolicy int

const (
	// policyOwnerAlive 归属存活性驱动：谓词判活则保留（含暂停待恢复等在途态）、判死则回收。
	// 任务属主根（download/share-receive）用——作用域键与属主行一一对应。
	policyOwnerAlive reclaimPolicy = iota
	// policyStartupPurge 启动一律回收：非任务入库是单次操作，进程重启即无在途。
	policyStartupPurge
	// policyDescriptionLedger 描述账本驱动：按描述登记的目标位置与临时文件名清单删除目标侧
	// 临时文件，清理落定后回收本作用域（目标侧删除真失败时保留作用域，下次启动凭账本重试；
	// 导出专用，不扫描用户盘）。
	policyDescriptionLedger
)

// rootEntry 根注册表登记项：属主（根名与属主常量同值）与回收策略的绑定。
type rootEntry struct {
	owner  Owner
	policy reclaimPolicy
}

// rootRegistry 根注册表。
var rootRegistry = []rootEntry{
	{owner: OwnerDownload, policy: policyOwnerAlive},
	{owner: OwnerShareReceive, policy: policyOwnerAlive},
	{owner: OwnerImport, policy: policyStartupPurge},
	{owner: OwnerMerge, policy: policyStartupPurge},
	{owner: OwnerExport, policy: policyDescriptionLedger},
}

// findRootEntry 按根目录名查注册表（属主常量值即根名）；未登记返回 nil。
func findRootEntry(rootName string) *rootEntry {
	for i := range rootRegistry {
		if string(rootRegistry[i].owner) == rootName {
			return &rootRegistry[i]
		}
	}
	return nil
}

// validateOwnerRegistered 校验属主已登记。
func validateOwnerRegistered(owner Owner) error {
	if findRootEntry(string(owner)) == nil {
		return fmt.Errorf("%w: %s", ErrOwnerNotRegistered, owner)
	}
	return nil
}
