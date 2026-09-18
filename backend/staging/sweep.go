package staging

// 统一清扫派发：按根注册表逐属主根裁决作用域去留。回收纪律：无描述/描述损坏/描述键与目录名
// 不一致的目录一律回收并记 Warn（含非空目录）；单个回收失败不中断其余目录（失败者留待下次
// 启动重试，幂等）；暂存总根下未登记的根名目录与非目录条目同样回收——暂存域是草稿区，不保
// 无主数据。

import (
	"context"
	"os"
	"path/filepath"

	"github.com/library-squirrel/backend/base/logger"
)

// removeAll 回收动作（os.RemoveAll 的可注入缝，测试以失败桩验证单败不中断）。
var removeAll = os.RemoveAll

// ScopeOwnerAlive 归属存活性谓词：作用域键 → 属主行是否仍存在。任务属主根（download/
// share-receive）的回收判定依据，由消费方在装配处注入（任务行存在性查询）；能力包不感知具体
// 属主表。谓词为 nil 时任务属主根下的作用域全部保留（无法判归属时宁留勿毁，记 Warn）。
type ScopeOwnerAlive func(scopeKey string) bool

// SweepAtStartup 启动清扫：遍历暂存总根下各登记属主根，按注册表策略逐作用域裁决——任务
// 属主根按谓词判活（活=保留〔含暂停待恢复〕，死=回收）、import/merge 根一律回收、export 根
// 按描述账本删目标目录临时文件，清理落定后回收本作用域（未落定保留作用域，下次启动凭账本
// 重试）。
// 时序契约：必须在启动序列中先于任何能创建作用域的服务启动处同步调用（app.go 装配保证）。
// workDir 空串=未配置，直接返回（启动期服务不启动的既有守卫语义）。
// 仅暂存总根不可读或 ctx 已取消时返回错误；各条目级失败均记日志不外抛。
func SweepAtStartup(ctx context.Context, workDir string, ownerAlive ScopeOwnerAlive) error {
	if workDir == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rootAbs := filepath.Join(workDir, RootName)
	entries, err := os.ReadDir(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		entryPath := filepath.Join(rootAbs, ent.Name())
		reg := findRootEntry(ent.Name())
		if reg == nil {
			reclaimAnomaly(entryPath, "未登记的暂存根")
			continue
		}
		if !ent.IsDir() {
			reclaimAnomaly(entryPath, "暂存总根下的非目录条目")
			continue
		}
		sweepOwnerRoot(ctx, entryPath, *reg, ownerAlive)
	}
	return nil
}

// sweepOwnerRoot 清扫单个属主根：逐条目读自证描述后按根策略裁决。
func sweepOwnerRoot(ctx context.Context, rootAbs string, reg rootEntry, ownerAlive ScopeOwnerAlive) {
	entries, err := os.ReadDir(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logger.Log.Warnf("[staging] 读取属主根目录失败（跳过，留待下次启动）: %s 错误=%v", rootAbs, err)
		return
	}
	for _, ent := range entries {
		if err := ctx.Err(); err != nil {
			return
		}
		scopePath := filepath.Join(rootAbs, ent.Name())
		if !ent.IsDir() {
			reclaimAnomaly(scopePath, "属主根下的非目录条目")
			continue
		}
		desc, derr := readScopeDescription(scopePath)
		if derr != nil {
			reclaimAnomaly(scopePath, derr.Error())
			continue
		}
		if desc.ScopeKey != ent.Name() {
			reclaimAnomaly(scopePath, "描述稳定键与目录名不一致")
			continue
		}
		switch reg.policy {
		case policyOwnerAlive:
			if ownerAlive == nil {
				logger.Log.Warnf("[staging] 归属谓词未注入，作用域保留待下次清扫: %s", scopePath)
				continue
			}
			if !ownerAlive(desc.ScopeKey) {
				reclaimByPolicy(scopePath, "属主行已消亡")
			}
		case policyStartupPurge:
			reclaimByPolicy(scopePath, "非任务属主启动清空")
		case policyDescriptionLedger:
			// 目标侧清理落定才回收作用域；未落定保留作用域（账本仍在）并继续处理其余目录，
			// 下次启动凭账本重试
			if cleanExportLedger(desc) {
				reclaimByPolicy(scopePath, "导出账本回收")
			}
		}
	}
}

// cleanExportLedger 按描述账本删除导出目标目录内的临时文件（描述即权威清单，不扫描目标目录）。
// 返回目标侧清理是否全部落定：删除成功与文件已不存在（目标目录整体不在即本无临时文件）均为
// 落定；删除真失败（目标盘不可达/文件被占用）为未落定，调用方须保留作用域供下次启动重试。
func cleanExportLedger(desc *ScopeDescription) bool {
	ledger := desc.Export
	if ledger == nil {
		// 账本缺失即目标位置不可知，目标盘临时文件成为孤儿（外部改动后果）；作用域目录本身
		// 仍照常回收
		logger.Log.Warnf("[staging] 导出作用域描述缺少账本，目标盘临时文件不可定位: 作用域=%s", desc.ScopeKey)
		return true
	}
	settled := true
	for _, name := range ledger.TempFiles {
		if !isBareFileName(name) {
			logger.Log.Warnf("[staging] 导出账本临时文件名非法，跳过该文件: %q", name)
			continue
		}
		target := filepath.Join(ledger.TargetDir, name)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			logger.Log.Warnf("[staging] 删除导出临时文件失败（作用域保留，下次启动凭账本重试）: %s 错误=%v", target, err)
			settled = false
		}
	}
	return settled
}

// reclaimAnomaly 异常态回收（无描述/描述损坏/未登记条目等）：成功与失败均记 Warn，失败不外抛。
func reclaimAnomaly(path, reason string) {
	if err := removeAll(path); err != nil {
		logger.Log.Warnf("[staging] 异常暂存条目回收失败（留待下次启动重试）: %s 原因=%s 错误=%v", path, reason, err)
		return
	}
	logger.Log.Warnf("[staging] 回收异常暂存条目: %s 原因=%s", path, reason)
}

// reclaimByPolicy 策略性回收（属主消亡/启动清空/账本回收）：成功记 Info，失败记 Warn 留待
// 下次启动重试，均不外抛。
func reclaimByPolicy(path, reason string) {
	if err := removeAll(path); err != nil {
		logger.Log.Warnf("[staging] 暂存作用域回收失败（留待下次启动重试）: %s 原因=%s 错误=%v", path, reason, err)
		return
	}
	logger.Log.Infof("[staging] 回收暂存作用域: %s 原因=%s", path, reason)
}
