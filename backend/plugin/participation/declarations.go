// Package participation 插件条目参与度真相层：per 插件会话（清单声明集 ⊕ resolver 覆盖表）
// 构成运行期唯一真相源，并承担 resolver 求值编排（读全量设置 → settingresolver 运行器 →
// shape 校验 → 覆盖表快照整体替换 → 条目级变更通知）。覆盖表零持久化——每会话激活相位
// 末尾由持久 KV 重算重建。
package participation

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/plugin/settingresolver"
)

// resolverContractVersion 当前唯一受支持的 resolver 契约版本（清单 settingsResolver.contractVersion
// 与脚本输出 version 字段的合法取值）
const resolverContractVersion = 1

// ErrResolverInvalid 安装期 settingsResolver 声明校验不合格（不合格项与期望形态见错误文本）
var ErrResolverInvalid = errors.New("插件 settingsResolver 声明校验失败")

// entryKey 条目键：条目所属派生面 + 条目 id（各面条目身份的最小载体）
type entryKey struct {
	Point settingresolver.Point
	ID    string
}

// declaredEntries 从清单派生声明集：插件在各派生面上声明的条目键全集。覆盖表只能作用于
// 声明集内条目的参与开关——声明集外条目凭空造不出（账 = 清单、态 = 覆盖，两相分离）
func declaredEntries(manifest *dto.PluginManifest) map[entryKey]struct{} {
	set := make(map[entryKey]struct{})
	if manifest == nil || manifest.Extensions == nil {
		return set
	}
	ext := manifest.Extensions
	for _, e := range ext.WorkFetch {
		set[entryKey{Point: settingresolver.PointWorkFetch, ID: e.ID}] = struct{}{}
	}
	for _, e := range ext.SiteAuthorFetch {
		set[entryKey{Point: settingresolver.PointSiteAuthorFetch, ID: e.ID}] = struct{}{}
	}
	for _, e := range ext.SiteBrowsers {
		set[entryKey{Point: settingresolver.PointSiteBrowsers, ID: e.ID}] = struct{}{}
	}
	for _, e := range ext.ResourceTypes {
		set[entryKey{Point: settingresolver.PointResourceTypes, ID: e.Type}] = struct{}{}
	}
	for _, e := range ext.FrontendExtensions {
		set[entryKey{Point: settingresolver.PointFrontendExtensions, ID: e.ID}] = struct{}{}
	}
	return set
}

// settingDefaults 清单根级设置声明 → 键到默认值。求值输入与设置页读取同构：
// KV 缺键回落声明默认值
func settingDefaults(manifest *dto.PluginManifest) map[string]string {
	defaults := make(map[string]string)
	if manifest == nil {
		return defaults
	}
	for _, d := range manifest.Settings {
		defaults[d.Key] = d.Default
	}
	return defaults
}

// ValidateResolverForInstall 安装期闸门（清单校验簇同层）：清单声明 settingsResolver 时
// 校验脚本工件与 dry-run 输出，不合格拒收安装；未声明时零行为（返回 nil）。判据：
//   - script 为非空裸文件名（不含路径分隔符，脚本工件住插件包根目录）
//   - contractVersion 为受支持值（当前唯一 1）
//   - scriptBytes（安装包内该名字文件的内容）在场且未超体积上限
//   - 语法可解析，且以声明的设置默认值 dry-run 一次，输出过 shape 校验：
//     运行器单条拒收（越界 point/非布尔 active 等非零即不合格；
//     声明集外条目同为不合格——覆盖不得作用于未声明条目
//
// scriptBytes 为 nil 表示安装包内无该文件
func ValidateResolverForInstall(manifest *dto.PluginManifest, scriptBytes []byte) error {
	if manifest == nil || manifest.SettingsResolver == nil {
		return nil
	}
	decl := manifest.SettingsResolver
	if decl.Script == "" || decl.Script != path.Base(decl.Script) || isDirShortcutName(decl.Script) {
		return fmt.Errorf("%w: script 须为非空裸文件名（住插件包根目录，不含路径分隔符）: %q",
			ErrResolverInvalid, decl.Script)
	}
	if decl.ContractVersion != resolverContractVersion {
		return fmt.Errorf("%w: contractVersion 须为 %d（唯一受支持的 resolver 契约版本），实际 %d",
			ErrResolverInvalid, resolverContractVersion, decl.ContractVersion)
	}
	if len(scriptBytes) == 0 {
		return fmt.Errorf("%w: 安装包内缺少 resolver 脚本文件 %s", ErrResolverInvalid, decl.Script)
	}

	declared := declaredEntries(manifest)
	defaults := settingDefaults(manifest)
	input := make(map[string]interface{}, len(defaults))
	for key, def := range defaults {
		input[key] = def
	}
	result, err := settingresolver.NewRunner().Evaluate(context.Background(), string(scriptBytes), settingresolver.Input{Settings: input})
	if err != nil {
		return fmt.Errorf("%w: 脚本以声明默认值 dry-run 失败: %v", ErrResolverInvalid, err)
	}
	if len(result.Rejected) > 0 {
		reasons := make([]string, 0, len(result.Rejected))
		for _, r := range result.Rejected {
			reasons = append(reasons, fmt.Sprintf("第 %d 条: %s", r.Index, r.Reason))
		}
		return fmt.Errorf("%w: dry-run 输出含不合格条目（越界 point/非布尔 active 等）: %s",
			ErrResolverInvalid, strings.Join(reasons, "；"))
	}
	for _, e := range result.Entries {
		if _, ok := declared[entryKey{Point: e.Point, ID: e.ID}]; !ok {
			return fmt.Errorf("%w: dry-run 输出条目 %s/%s 不在清单声明集内（覆盖只能作用于已声明条目）",
				ErrResolverInvalid, e.Point, e.ID)
		}
	}
	return nil
}

// isDirShortcutName 判断文件名是否为目录捷径名（"." 或 ".."——path.Base 对二者原样返回，
// 须单独排除）
func isDirShortcutName(name string) bool {
	return name == "." || name == ".."
}
