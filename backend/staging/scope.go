package staging

// 作用域自证描述（scope.json）：承载稳定键、内容形态、创建时刻，导出形态另记目标位置与临时
// 文件名账本。作用域创建只能经本文件的统一入口（CreateScope/CreateExportScope）：先建临时名
// 目录、在其内写好描述、再 rename 为正式名（同卷目录 rename 原子），故「目录存在 ⟺ 描述在」
// 恒成立；无描述/描述损坏的目录只能来自外部改动，清扫侧按无主数据回收。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// scopeDescFileName 作用域描述文件名（每个作用域目录内一份）。
const scopeDescFileName = "scope.json"

// tempScopeDirPrefix 创建期临时目录名前缀；rename 前的崩溃残留会被启动清扫按无描述目录回收。
const tempScopeDirPrefix = ".tmp-"

// scopeKeyPattern 作用域键字符集：1-128 位字母/数字/下划线/连字符。任务 ID（纯数字）与铸造键
// （hex）均在其内；排除路径分隔符与点号，保证键是安全的单段目录名。
var scopeKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ScopeDescription 作用域自证描述（目录内 scope.json 的内容）。
type ScopeDescription struct {
	// ScopeKey 作用域稳定键，与目录名一致（清扫时核对，不一致按描述损坏回收）。任务属主根下
	// =任务 ID 字符串；非任务属主根下=铸造键（MintScopeKey）。
	ScopeKey string `json:"scopeKey"`
	// ContentShape 内容形态标签：目录内文件的布局形态，由创建方定义并自行消费（如按文件键
	// 平铺、含清单的父子镜像树），能力包只存取不解释。
	ContentShape string `json:"contentShape"`
	// CreatedAt 创建时刻（Unix 毫秒时间戳）。
	CreatedAt int64 `json:"createdAt"`
	// Export 导出形态专用账本；其他属主恒为 nil。
	Export *ExportLedger `json:"export,omitempty"`
}

// ExportLedger 导出临时文件账本：描述即权威清单，回收按账本执行、不扫描目标目录。
type ExportLedger struct {
	// TargetDir 导出目标目录（绝对路径，用户自选，可在任意盘）。
	TargetDir string `json:"targetDir"`
	// TempFiles 目标目录内的临时文件名清单（纯文件名，不含路径分隔符，可为空——导出未及写
	// 任何临时文件即退出的场景）。
	TempFiles []string `json:"tempFiles"`
}

// MintScopeKey 铸造非任务属主的作用域稳定键：16 字节随机数 hex（32 字符）。非计数器——无需
// DB 归属即可判唯一（一个 workDir 恒对应一份 DB，不存在需要代际标识的场景），随机性保证两次
// 铸造不撞键。
func MintScopeKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("铸造作用域键失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// CreateScope 创建作用域（唯一合法入口）：在属主根下原子化建目录并写入自证描述，返回作用域
// 目录绝对路径（absPath 域，仅供 os.* 文件系统调用点现场消费）。同键目录已存在时返回
// ErrScopeExists。暂存内容文件由创建方随后写入返回的目录内。
func CreateScope(ctx context.Context, workDir string, owner Owner, scopeKey, contentShape string) (string, error) {
	if workDir == "" {
		return "", ErrWorkDirEmpty
	}
	if err := validateScopeIdentity(owner, scopeKey, contentShape); err != nil {
		return "", err
	}
	desc := ScopeDescription{
		ScopeKey:     scopeKey,
		ContentShape: contentShape,
		CreatedAt:    time.Now().UnixMilli(),
	}
	return materializeScope(workDir, owner, desc)
}

// CreateExportScope 创建导出作用域：目录内只放描述，物理临时文件由导出执行面写在目标目录
// 同级，其位置与名字记入账本供回收。正常链路由执行面完成/失败时自删，启动清扫兜底崩溃残留
// （目标盘不可达容忍失败，留待下次启动重试）。
func CreateExportScope(ctx context.Context, workDir, scopeKey, contentShape string, ledger ExportLedger) (string, error) {
	if workDir == "" {
		return "", ErrWorkDirEmpty
	}
	if err := validateScopeIdentity(OwnerExport, scopeKey, contentShape); err != nil {
		return "", err
	}
	if ledger.TargetDir == "" {
		return "", ErrEmptyExportTargetDir
	}
	for _, name := range ledger.TempFiles {
		if !isBareFileName(name) {
			return "", fmt.Errorf("%w: %q", ErrInvalidTempFileName, name)
		}
	}
	desc := ScopeDescription{
		ScopeKey:     scopeKey,
		ContentShape: contentShape,
		CreatedAt:    time.Now().UnixMilli(),
		Export:       &ledger,
	}
	return materializeScope(workDir, OwnerExport, desc)
}

// validateScopeIdentity 创建入口共用的身份校验：属主已登记、键为安全单段目录名、内容形态非空。
func validateScopeIdentity(owner Owner, scopeKey, contentShape string) error {
	if err := validateOwnerRegistered(owner); err != nil {
		return err
	}
	if err := validateScopeKey(scopeKey); err != nil {
		return err
	}
	if contentShape == "" {
		return ErrEmptyContentShape
	}
	return nil
}

// materializeScope 落盘作用域目录：临时名目录内写好描述 → rename 正式名。任一步失败即回收
// 临时目录，不留半成品。
func materializeScope(workDir string, owner Owner, desc ScopeDescription) (string, error) {
	rootAbs := ownerRootPath(workDir, owner)
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		return "", fmt.Errorf("创建属主根目录失败: %w", err)
	}
	finalDir := filepath.Join(rootAbs, desc.ScopeKey)
	if _, err := os.Lstat(finalDir); err == nil {
		return "", fmt.Errorf("%w: %s", ErrScopeExists, desc.ScopeKey)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查作用域目录失败: %w", err)
	}
	tempDir, err := os.MkdirTemp(rootAbs, tempScopeDirPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("创建临时作用域目录失败: %w", err)
	}
	data, err := json.MarshalIndent(desc, "", "  ")
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("序列化作用域描述失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, scopeDescFileName), data, 0o644); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("写入作用域描述失败: %w", err)
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("作用域目录定名失败: %w", err)
	}
	return finalDir, nil
}

// readScopeDescription 读取并校验作用域目录内的自证描述。文件缺失、JSON 不可解析或字段校验
// 不过均返回错误——清扫侧对这三种形态一律按「描述不在」回收。
func readScopeDescription(scopeDir string) (*ScopeDescription, error) {
	data, err := os.ReadFile(filepath.Join(scopeDir, scopeDescFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrScopeDescMissing
		}
		return nil, fmt.Errorf("读取作用域描述失败: %w", err)
	}
	var desc ScopeDescription
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScopeDescCorrupt, err)
	}
	if err := validateScopeKey(desc.ScopeKey); err != nil {
		return nil, fmt.Errorf("%w: %q", ErrScopeDescCorrupt, desc.ScopeKey)
	}
	if desc.ContentShape == "" {
		return nil, fmt.Errorf("%w: 内容形态为空", ErrScopeDescCorrupt)
	}
	return &desc, nil
}

// validateScopeKey 校验作用域键为安全单段目录名。
func validateScopeKey(key string) error {
	if !scopeKeyPattern.MatchString(key) {
		return fmt.Errorf("%w: %q", ErrInvalidScopeKey, key)
	}
	return nil
}

// isBareFileName 纯文件名判定：非空、不含路径分隔符、非目录游标——路径拼接前的输入校验，
// 拒绝携带相对路径段的账本值。
func isBareFileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}
