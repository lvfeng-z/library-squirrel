// genofficial 官方插件指纹名单生成工具（构建管线专用，属 build/ 脚手架域，不入 go build ./... 全量）。
// 输入成功构建的插件 zip 包：读包内 plugin.json 取 publicId/buildId、复用 backend/plugin 的
// 内容摘要实现算 contentDigest，upsert 进订死配置 backend/config/locked_config.yaml。
// 名单文件为机器生成域：整体读-改-写（解析 → 合并条目 → 固定文件头注释 + marshal 重写），无段替换手术。
//
// 官方化是显式声明（2026-11-12 用户裁决）：打包（task build:plugins）默认不再触碰名单，
// 仅 -Official/手动本工具显式官方化；名单语义=「受认可的官方发布」，dirty 构建（源状态不被
// 任何 git 提交捕获、不可复现不可审计）默认拒绝入名单，-allow-dirty 留显式出口。
//
// 用法：go run ./build/tools/genofficial -zip <package.zip> [-locked <locked_config.yaml>] [-allow-dirty]
//       go run ./build/tools/genofficial -prune-dirty [-locked <locked_config.yaml>]
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/config"
	"github.com/library-squirrel/backend/plugin"
	yaml "go.yaml.in/yaml/v3"
)

// lockedHeader 订死配置文件头注释（固定文本：类别语义说明，随整体重写原样保留）
const lockedHeader = `# 订死配置（locked）——随二进制分发的权威数据，不参与两层合并，磁盘无对应覆盖物；
# 由构建管线 task build:plugins 生成维护，勿手改（手改仅影响本地构建产物）`

// readManifestFromZip 从插件包内读取 plugin.json 并解析（身份与构建标识取自包自声明，仅作定位键）
func readManifestFromZip(packagePath string) (*dto.PluginManifest, error) {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return nil, fmt.Errorf("打开插件包失败: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "plugin.json" || file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("读取包内 plugin.json 失败: %w", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("读取包内 plugin.json 失败: %w", err)
		}
		var manifest dto.PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("解析 plugin.json 失败: %w", err)
		}
		return &manifest, nil
	}
	return nil, fmt.Errorf("包内缺少 plugin.json: %s", packagePath)
}

// upsertEntry 名单条目合并：同 (publicId, buildId) 键的既有条目一律摘除、由新条目统一代表
// （同键多录收敛为一），新键追加。返回合并后名单与合并结果：
// append=新键追加 / match=同键且摘要一致（幂等命中）/ overwrite=同键但摘要不同（按最新管线产出覆盖，
// 同 buildId 跨机构建字节漂移的冲突形态，调用方负责警示）
func upsertEntry(entries []config.OfficialPluginEntry, entry config.OfficialPluginEntry) ([]config.OfficialPluginEntry, string) {
	seen := false
	match := false
	kept := make([]config.OfficialPluginEntry, 0, len(entries)+1)
	for _, e := range entries {
		if e.PublicID == entry.PublicID && e.BuildID == entry.BuildID {
			seen = true
			if e.ContentDigest == entry.ContentDigest {
				match = true
			}
			continue
		}
		kept = append(kept, e)
	}
	kept = append(kept, entry)
	// 按 (publicId, buildId) 排序保证输出与构建顺序无关，重复运行字节级稳定
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].PublicID != kept[j].PublicID {
			return kept[i].PublicID < kept[j].PublicID
		}
		return kept[i].BuildID < kept[j].BuildID
	})
	switch {
	case !seen:
		return kept, "append"
	case match:
		return kept, "match"
	default:
		return kept, "overwrite"
	}
}

// marshalLocked 序列化订死配置：固定文件头注释 + yaml 缩进 2。空名单落 `officialPlugins: []`
func marshalLocked(entries []config.OfficialPluginEntry) ([]byte, error) {
	if entries == nil {
		entries = []config.OfficialPluginEntry{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(config.LockedConfig{OfficialPlugins: entries}); err != nil {
		return nil, fmt.Errorf("序列化订死配置失败: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("序列化订死配置失败: %w", err)
	}
	return []byte(lockedHeader + "\n" + buf.String()), nil
}

// run 单包处理主流程：取身份键 → 算摘要 → 读-改-写名单文件。
// 输出与磁盘内容字节一致时跳过写入（幂等，不扰动文件时间戳）。
// dirty 守卫：buildId 含 "-dirty"（源状态不被 git 提交捕获）默认拒绝——官方名单=受认可
// 发布的注册表，不可复现产物不具官方资格；allowDirty 为显式出口（如临时认可一个未提交修复）
func run(zipPath, lockedPath string, allowDirty bool) error {
	manifest, err := readManifestFromZip(zipPath)
	if err != nil {
		return err
	}
	if manifest.ID == "" || manifest.BuildID == "" {
		return fmt.Errorf("plugin.json 缺少 id 或 buildId（包：%s），构建管线打标缺失", zipPath)
	}
	if !allowDirty && strings.Contains(manifest.BuildID, "-dirty") {
		return fmt.Errorf("buildId=%s 为 dirty 构建（源状态不被任何 git 提交捕获），默认拒绝入官方名单；"+
			"先提交干净树重打，或确需临时认可时加 -allow-dirty 显式覆盖", manifest.BuildID)
	}
	digest, err := plugin.ComputeContentDigestZip(zipPath)
	if err != nil {
		return fmt.Errorf("计算内容摘要失败: %w", err)
	}
	entry := config.OfficialPluginEntry{PublicID: manifest.ID, BuildID: manifest.BuildID, ContentDigest: digest}

	data, err := os.ReadFile(lockedPath)
	if err != nil {
		return fmt.Errorf("读取订死配置失败: %w", err)
	}
	var locked config.LockedConfig
	if err := yaml.Unmarshal(data, &locked); err != nil {
		return fmt.Errorf("解析订死配置失败（机器生成域，损坏须重新生成）: %w", err)
	}

	merged, outcome := upsertEntry(locked.OfficialPlugins, entry)
	switch outcome {
	case "match":
		fmt.Printf("[genofficial] 名单条目已在且摘要一致（无改动）: %s (buildId=%s)\n", entry.PublicID, entry.BuildID)
	case "append":
		fmt.Printf("[genofficial] 名单新增条目: %s (buildId=%s, digest=%s)\n", entry.PublicID, entry.BuildID, entry.ContentDigest)
	default:
		fmt.Printf("[genofficial] 警告：同 (publicId, buildId) 条目摘要不同，按最新管线产出覆盖: %s (buildId=%s, digest=%s)\n",
			entry.PublicID, entry.BuildID, entry.ContentDigest)
	}

	out, err := marshalLocked(merged)
	if err != nil {
		return err
	}
	if bytes.Equal(data, out) {
		return nil
	}
	if err := os.WriteFile(lockedPath, out, 0o644); err != nil {
		return fmt.Errorf("写回订死配置失败: %w", err)
	}
	return nil
}

// pruneDirty 清淤模式：剔除名单中全部 dirty 构建条目（buildId 含 "-dirty"）。
// 测试指纹入名单是旧管线的自动副作用（2026-11-12 前无条件 upsert），此模式一次性回收
func pruneDirty(lockedPath string) error {
	data, err := os.ReadFile(lockedPath)
	if err != nil {
		return fmt.Errorf("读取订死配置失败: %w", err)
	}
	var locked config.LockedConfig
	if err := yaml.Unmarshal(data, &locked); err != nil {
		return fmt.Errorf("解析订死配置失败（机器生成域，损坏须重新生成）: %w", err)
	}
	kept := make([]config.OfficialPluginEntry, 0, len(locked.OfficialPlugins))
	removed := 0
	for _, e := range locked.OfficialPlugins {
		if strings.Contains(e.BuildID, "-dirty") {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	out, err := marshalLocked(kept)
	if err != nil {
		return err
	}
	if bytes.Equal(data, out) {
		fmt.Printf("[genofficial] 无 dirty 条目可清（幂等，文件未动）: %s\n", lockedPath)
		return nil
	}
	if err := os.WriteFile(lockedPath, out, 0o644); err != nil {
		return fmt.Errorf("写回订死配置失败: %w", err)
	}
	fmt.Printf("[genofficial] 已剔除 dirty 条目 %d 条，余 %d 条: %s\n", removed, len(kept), lockedPath)
	return nil
}

// clearRoster 清空模式：名单归零（保留文件头与空列表形态，与 marshalLocked 产出一致）。
// 2026-11-12 用户裁决：当前无正式发布插件，历史条目均为开发期交付产物——名单清空回到
// 与现实一致的空态，自首个正式发布（build:plugins:official）重新起算；历史指纹在 git
// 提交史中留档不丢
func clearRoster(lockedPath string) error {
	data, err := os.ReadFile(lockedPath)
	if err != nil {
		return fmt.Errorf("读取订死配置失败: %w", err)
	}
	var locked config.LockedConfig
	if err := yaml.Unmarshal(data, &locked); err != nil {
		return fmt.Errorf("解析订死配置失败（机器生成域，损坏须重新生成）: %w", err)
	}
	out, err := marshalLocked(nil)
	if err != nil {
		return err
	}
	if bytes.Equal(data, out) {
		fmt.Printf("[genofficial] 名单已为空（幂等，文件未动）: %s\n", lockedPath)
		return nil
	}
	if err := os.WriteFile(lockedPath, out, 0o644); err != nil {
		return fmt.Errorf("写回订死配置失败: %w", err)
	}
	fmt.Printf("[genofficial] 名单已清空（移除 %d 条，历史在 git 提交史留档）: %s\n", len(locked.OfficialPlugins), lockedPath)
	return nil
}

func main() {
	zipPath := flag.String("zip", "", "插件安装包 zip 路径（-prune-dirty 模式下可省）")
	lockedPath := flag.String("locked", "backend/config/locked_config.yaml", "订死配置文件路径（相对主仓库根）")
	allowDirty := flag.Bool("allow-dirty", false, "显式覆盖 dirty 守卫：允许 dirty 构建入名单（临时认可未提交产物的出口）")
	pruneDirtyMode := flag.Bool("prune-dirty", false, "清淤模式：剔除名单中全部 dirty 条目（无需 -zip）")
	clearMode := flag.Bool("clear", false, "清空模式：名单归零（无正式发布时的空态重置，历史在 git 留档；无需 -zip）")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "官方插件指纹名单生成工具：读插件包产出 (publicId, buildId, contentDigest) 条目并 upsert 进订死配置")
		fmt.Fprintln(os.Stderr, "官方化是显式声明：打包（task build:plugins）默认不动名单；dirty 构建默认拒绝入名单")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "用法: go run ./build/tools/genofficial -zip <package.zip> [-locked <locked_config.yaml>] [-allow-dirty]")
		fmt.Fprintln(os.Stderr, "      go run ./build/tools/genofficial -prune-dirty [-locked <locked_config.yaml>]")
		fmt.Fprintln(os.Stderr, "      go run ./build/tools/genofficial -clear [-locked <locked_config.yaml>]")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *clearMode {
		if err := clearRoster(*lockedPath); err != nil {
			fmt.Fprintf(os.Stderr, "[genofficial] 失败: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *pruneDirtyMode {
		if err := pruneDirty(*lockedPath); err != nil {
			fmt.Fprintf(os.Stderr, "[genofficial] 失败: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *zipPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*zipPath, *lockedPath, *allowDirty); err != nil {
		fmt.Fprintf(os.Stderr, "[genofficial] 失败: %v\n", err)
		os.Exit(1)
	}
}
