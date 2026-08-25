package plugin

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/library-squirrel/backend/base/logger"
	domain "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/config"
)

// fileDigestPair 内容摘要收集单元：文件相对路径（正斜杠）与其内容 sha256 hex
type fileDigestPair struct {
	Path   string
	Digest string
}

// aggregateContentDigest 内容摘要聚合算法：按路径字典序排序 → 逐条拼接 "{path}\n{sha256hex}\n"
// → 对拼接全文再 sha256。输入只含文件条目，路径统一正斜杠——zip 容器元数据（entry mtime、压缩参数）
// 不参与，同内容重打包摘要不变
func aggregateContentDigest(pairs []fileDigestPair) string {
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Path < pairs[j].Path })
	var sb strings.Builder
	for _, p := range pairs {
		sb.WriteString(p.Path)
		sb.WriteByte('\n')
		sb.WriteString(p.Digest)
		sb.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// contentDigestZip 计算插件 zip 包的内容摘要：读全部文件 entry 流（不解压落盘），目录 entry 不参与
func contentDigestZip(packagePath string) (string, error) {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return "", fmt.Errorf("打开插件包失败: %w", err)
	}
	defer reader.Close()

	pairs := make([]fileDigestPair, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("读取包内文件 %s 失败: %w", file.Name, err)
		}
		h := sha256.New()
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("计算包内文件 %s 摘要失败: %w", file.Name, err)
		}
		pairs = append(pairs, fileDigestPair{
			Path:   filepath.ToSlash(file.Name),
			Digest: hex.EncodeToString(h.Sum(nil)),
		})
	}
	return aggregateContentDigest(pairs), nil
}

// ComputeContentDigestZip 插件 zip 包内容摘要的导出入口：构建管线的官方指纹名单生成工具
// （build/tools/genofficial）与运行时官方判定共用同一实现，消除生成端与校验端的算法分叉
func ComputeContentDigestZip(packagePath string) (string, error) {
	return contentDigestZip(packagePath)
}

// contentDigestDir 计算已解压插件目录的内容摘要：walk 全部文件（供存量已装目录证实，与 zip 源产出同一摘要）
func contentDigestDir(dir string) (string, error) {
	var pairs []fileDigestPair
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		pairs = append(pairs, fileDigestPair{
			Path:   filepath.ToSlash(rel),
			Digest: hex.EncodeToString(sum[:]),
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("遍历插件目录失败: %w", err)
	}
	return aggregateContentDigest(pairs), nil
}

// officialRoster 官方指纹名单（nil 安全：config 未加载或空名单返回 nil，判定全不命中）
func officialRoster() []config.OfficialPluginEntry {
	if config.Get() == nil {
		return nil
	}
	return config.Get().Locked.OfficialPlugins
}

// digestCandidates 名单中 (publicId, buildId) 双键命中的条目内容摘要集合。
// buildId 不等则内容必不等（buildId 是源码状态的函数），可作终裁前短路；同键多录时全参与比对
func digestCandidates(entries []config.OfficialPluginEntry, publicId, buildId string) []string {
	if publicId == "" || buildId == "" {
		return nil
	}
	var digests []string
	for _, e := range entries {
		if e.PublicID == publicId && e.BuildID == buildId && e.ContentDigest != "" {
			digests = append(digests, e.ContentDigest)
		}
	}
	return digests
}

// matchOfficialZip 官方指纹名单终裁（zip 包源，安装路径用）：
// publicId 无条目或 buildId 不在其条目集 → false（短路，省包级摘要计算）；
// buildId 命中后算包内容摘要比对条目。摘要计算失败保守 false——官方身份是展示维度，不拦安装
func matchOfficialZip(entries []config.OfficialPluginEntry, publicId, buildId, packagePath string) bool {
	candidates := digestCandidates(entries, publicId, buildId)
	if len(candidates) == 0 {
		return false
	}
	digest, err := contentDigestZip(packagePath)
	if err != nil {
		logger.Log.Warnf("计算插件包内容摘要失败（官方身份保守未证实）: %s, %v", packagePath, err)
		return false
	}
	return containsDigest(candidates, digest)
}

// matchOfficialDir 官方指纹名单终裁（已解压目录源，存量证实用），比对逻辑与 matchOfficialZip 一致
func matchOfficialDir(entries []config.OfficialPluginEntry, publicId, buildId, dir string) bool {
	candidates := digestCandidates(entries, publicId, buildId)
	if len(candidates) == 0 {
		return false
	}
	digest, err := contentDigestDir(dir)
	if err != nil {
		logger.Log.Warnf("计算已装插件目录内容摘要失败（存量官方证实跳过）: %s, %v", dir, err)
		return false
	}
	return containsDigest(candidates, digest)
}

// containsDigest 摘要集合包含判定
func containsDigest(candidates []string, digest string) bool {
	for _, c := range candidates {
		if c == digest {
			return true
		}
	}
	return false
}

// matchOfficial 判定安装包是否命中官方指纹名单（安装路径的官方身份唯一推导点入参）。
// 渠道不短路：bundled 安装同样查名单（config.yaml 清单被换自制 zip 时装出的行 source=bundled 但 official=false）
func (s *Service) matchOfficial(installDTO *domain.PluginInstallDTO) sql.NullBool {
	return sql.NullBool{Bool: matchOfficialZip(officialRoster(), installDTO.PublicID, installDTO.BuildID, installDTO.PackagePath), Valid: true}
}

// verifyExistingOfficial 存量已装行官方身份证实（启动扫描顺带执行）：existing.Official 非 true 且
// buildId 命中名单时，对已装目录按目录源算内容摘要比对名单，命中则 Updates 置 official=true。
// 证实的是历史事实（落库 true 后不随主程序升级翻转）；已 true 短路幂等，未打标历史包不证实。
// installedDir 为已装目录绝对路径（RootPath 无效时传空串）
func (s *Service) verifyExistingOfficial(ctx context.Context, entries []config.OfficialPluginEntry, existing *entity2.Plugin, installedDir string) {
	if existing == nil {
		return
	}
	if existing.Official.Valid && existing.Official.Bool {
		return
	}
	if !existing.PublicID.Valid || existing.PublicID.String == "" ||
		!existing.BuildID.Valid || existing.BuildID.String == "" || installedDir == "" {
		return
	}
	if !matchOfficialDir(entries, existing.PublicID.String, existing.BuildID.String, installedDir) {
		return
	}
	existing.Official = sql.NullBool{Bool: true, Valid: true}
	if err := s.repo.Updates(ctx, existing); err != nil {
		logger.Log.Warnf("存量插件官方身份证实落库失败: %s, %v", existing.PublicID.String, err)
		return
	}
	logger.Log.Infof("存量插件经内容摘要证实为官方发布产物: %s (buildId=%s)", existing.PublicID.String, existing.BuildID.String)
}
