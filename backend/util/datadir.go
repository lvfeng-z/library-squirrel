package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// —— 用户数据目录解析（三目录契约之「用户数据目录」）——
//
// 优先级（高到低）：
//  1. --data-dir 命令行参数（--data-dir VALUE 或 --data-dir=VALUE，取首个匹配）
//  2. LS_DATA_DIR 环境变量（空串视为未设置）
//  3. APP_ENV=development 开发模式：源码树根（与 RootPath() 开发分支同源——
//     go run / wails dev 自模块根启动时 Getwd 即源码树根，开发工作流零变化）
//  4. %LOCALAPPDATA%\library-squirrel（Windows 本机应用数据标准位置，不随域漫游）
//
// 边界约定：
//   - 解析器只依赖 os 与环境变量，不依赖 GUI/WebView 初始化——无 GUI 前置的调用方
//     （测试基建、多开实例的数据目录覆盖）走同一入口；
//   - 只解析根目录并 MkdirAll，不预建 config.yaml、config/、database/、log/、
//     plugin/、webview2/ 等内容条目（由各使用方自建）；
//   - RootPath() 语义保留为「程序目录」专用（exe 所在目录），与本解析器职责分离。

const (
	// DataDirEnvVar 数据目录覆盖环境变量——多库多开与测试基建实例隔离的注入通道
	DataDirEnvVar = "LS_DATA_DIR"

	// dataDirDefaultName 默认用户数据目录名（%LOCALAPPDATA% 之下的目录名）
	dataDirDefaultName = "library-squirrel"

	// dataDirFlag 数据目录命令行参数名
	dataDirFlag = "--data-dir"
)

// ErrDataDirNotAbsolute 显式覆盖（--data-dir 参数或 LS_DATA_DIR 环境变量）给了
// 相对路径——显式参数须绝对路径，早失败优于歧义解析（静默按进程工作目录相对化
// 会产生歧义落点）
var ErrDataDirNotAbsolute = errors.New("data dir override must be an absolute path")

// ErrLocalAppDataUndefined %LOCALAPPDATA% 未定义（极端环境）——不静默回退到
// 其他位置，要求显式指定
var ErrLocalAppDataUndefined = errors.New("LOCALAPPDATA environment variable is not defined")

// 进程内一次解析、全局缓存。失败结果同样固化（首解错误后不再重试）：调用方对
// 返回错误应做启动期致命处理，进程随后退出，不存在「修复后重试」的场景。
var (
	dataDirOnce sync.Once
	dataDirPath string
	dataDirErr  error
)

// DataDir 返回用户数据目录绝对路径。首次调用完成解析并缓存，之后调用零开销且
// 结果不变（并发安全）。--data-dir 参数由本函数自行扫描 os.Args（--data-dir
// VALUE 与 --data-dir=VALUE 两种形态），调用方无需预先解析。
// 解析成功包含 MkdirAll 创建根目录（已存在则无操作）；失败返回带可读提示的
// 致命错误。
func DataDir() (string, error) {
	dataDirOnce.Do(func() {
		dataDirPath, dataDirErr = resolveDataDir(os.Args)
	})
	return dataDirPath, dataDirErr
}

// resolveDataDir 核心解析（纯 os/env 逻辑，无缓存、无 GUI 依赖）。
func resolveDataDir(args []string) (string, error) {
	// 1. --data-dir 命令行参数
	if v, found := findDataDirArg(args); found {
		return establishDataDir(v, "--data-dir 参数")
	}
	// 2. LS_DATA_DIR 环境变量（空串视为未设置）
	if v := os.Getenv(DataDirEnvVar); v != "" {
		return establishDataDir(v, "环境变量 "+DataDirEnvVar)
	}
	// 3. 开发模式：源码树根
	if os.Getenv("APP_ENV") == "development" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("datadir: 开发模式获取源码树根失败: %w", err)
		}
		return establishDataDir(wd, "开发模式源码树根")
	}
	// 4. 默认：%LOCALAPPDATA%\library-squirrel（未定义或空串都视为未定义）
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", fmt.Errorf(
			"datadir: %%LOCALAPPDATA%% 未定义，无法定位默认用户数据目录；"+
				"可用 --data-dir 参数或 %s 环境变量显式指定绝对路径: %w",
			DataDirEnvVar, ErrLocalAppDataUndefined)
	}
	return establishDataDir(filepath.Join(base, dataDirDefaultName), "默认位置")
}

// establishDataDir 校验绝对路径、规范化并创建目录（MkdirAll）。source 仅用于
// 错误提示中标注路径来源。
func establishDataDir(dir, source string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("datadir: %s 需要绝对路径，得到 %q: %w", source, dir, ErrDataDirNotAbsolute)
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("datadir: 创建数据目录失败（来源: %s）%q: %w", source, dir, err)
	}
	return dir, nil
}

// findDataDirArg 扫描参数中的 --data-dir，支持「--data-dir VALUE」与
// 「--data-dir=VALUE」两种形态，取首个匹配。found=true 表示参数出现（即使值为
// 空——空值会因非绝对路径在 establishDataDir 被拒绝，早失败优于静默忽略）。
func findDataDirArg(args []string) (value string, found bool) {
	for i := range args {
		a := args[i]
		if a == dataDirFlag {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
		if after, ok := strings.CutPrefix(a, dataDirFlag+"="); ok {
			return after, true
		}
	}
	return "", false
}
