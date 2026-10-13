package util

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// —— 测试隔离手段 ——
//
// 优先级矩阵各用例共用进程级缓存与环境，隔离方式：
//   - 缓存：resetDataDirCache 直接重置同包未导出的缓存变量（每用例开头与收尾各一次）；
//   - 环境：setResolverEnv 用 t.Setenv 全量接管三个输入（APP_ENV / LS_DATA_DIR /
//     LOCALAPPDATA），空串与解析器的「按未设置处理」约定一致，用例互不污染、
//     也不受宿主环境影响；
//   - 默认分支：LOCALAPPDATA 指向 t.TempDir()，不触碰宿主真实 %LOCALAPPDATA%。

// resetDataDirCache 重置 DataDir 的进程级缓存（仅同包测试使用）
func resetDataDirCache() {
	dataDirOnce = sync.Once{}
	dataDirPath = ""
	dataDirErr = nil
}

// setResolverEnv 全量接管解析器的三个环境输入，各参数传空串即「按未设置」
func setResolverEnv(t *testing.T, appEnv, lsDataDir, localAppData string) {
	t.Helper()
	t.Setenv("APP_ENV", appEnv)
	t.Setenv("LS_DATA_DIR", lsDataDir)
	t.Setenv("LOCALAPPDATA", localAppData)
}

// assertDirCreated 断言路径存在且为目录（解析成功含 MkdirAll 的直接证据）
func assertDirCreated(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("数据目录未被创建: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("数据目录路径不是目录: %s", dir)
	}
}

// resolveOK 断言解析成功且返回期望路径（已创建）
func resolveOK(t *testing.T, got string, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("解析意外失败: %v", err)
	}
	if got != want {
		t.Fatalf("解析结果 = %q, 期望 %q", got, want)
	}
	assertDirCreated(t, got)
}

// —— 优先级矩阵：--data-dir 参数 > LS_DATA_DIR > 开发模式 > 默认 ——

func TestResolveDataDirPriority(t *testing.T) {
	t.Run("参数高于环境变量_开发模式_默认", func(t *testing.T) {
		base := t.TempDir()
		setResolverEnv(t, "development", filepath.Join(base, "from-env"), base)
		argDir := filepath.Join(base, "from-arg")
		got, err := resolveDataDir([]string{"app.exe", "--data-dir", argDir})
		resolveOK(t, got, err, argDir)
	})

	t.Run("参数等号形态", func(t *testing.T) {
		base := t.TempDir()
		setResolverEnv(t, "development", filepath.Join(base, "from-env"), base)
		argDir := filepath.Join(base, "from-arg")
		got, err := resolveDataDir([]string{"app.exe", "--data-dir=" + argDir, "--other"})
		resolveOK(t, got, err, argDir)
	})

	t.Run("环境变量高于开发模式", func(t *testing.T) {
		base := t.TempDir()
		envDir := filepath.Join(base, "from-env")
		setResolverEnv(t, "development", envDir, base)
		got, err := resolveDataDir([]string{"app.exe"})
		resolveOK(t, got, err, envDir)
	})

	t.Run("环境变量高于默认", func(t *testing.T) {
		base := t.TempDir()
		envDir := filepath.Join(base, "from-env")
		setResolverEnv(t, "", envDir, base)
		got, err := resolveDataDir([]string{"app.exe"})
		resolveOK(t, got, err, envDir)
	})

	t.Run("开发模式取源码树根", func(t *testing.T) {
		base := t.TempDir()
		setResolverEnv(t, "development", "", base)
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("获取工作目录失败: %v", err)
		}
		got, err := resolveDataDir([]string{"app.exe"})
		resolveOK(t, got, err, filepath.Clean(wd))
	})

	t.Run("默认位置", func(t *testing.T) {
		base := t.TempDir()
		setResolverEnv(t, "", "", base)
		want := filepath.Join(base, dataDirDefaultName)
		got, err := resolveDataDir([]string{"app.exe"})
		resolveOK(t, got, err, want)
		// 本阶段只建根目录，不预建 config/database/log/plugin/webview2 等内容条目
		entries, err := os.ReadDir(got)
		if err != nil {
			t.Fatalf("读取数据目录失败: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("解析器不应预建子条目, 实际存在: %v", entries)
		}
	})
}

// —— 失败路径：早失败、错误可判别、不吞 ——

func TestResolveDataDirFailures(t *testing.T) {
	t.Run("参数给相对路径", func(t *testing.T) {
		setResolverEnv(t, "", "", t.TempDir())
		got, err := resolveDataDir([]string{"app.exe", "--data-dir", "relative/dir"})
		if got != "" {
			t.Fatalf("相对路径应无返回值, 得到 %q", got)
		}
		if !errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("期望 ErrDataDirNotAbsolute, 得到: %v", err)
		}
	})

	t.Run("参数等号形态给相对路径", func(t *testing.T) {
		setResolverEnv(t, "", "", t.TempDir())
		_, err := resolveDataDir([]string{"app.exe", "--data-dir=relative"})
		if !errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("期望 ErrDataDirNotAbsolute, 得到: %v", err)
		}
	})

	t.Run("参数缺值", func(t *testing.T) {
		setResolverEnv(t, "", "", t.TempDir())
		_, err := resolveDataDir([]string{"app.exe", "--data-dir"})
		if !errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("缺值应按空值拒绝, 期望 ErrDataDirNotAbsolute, 得到: %v", err)
		}
	})

	t.Run("环境变量给相对路径", func(t *testing.T) {
		setResolverEnv(t, "", "relative/env", t.TempDir())
		got, err := resolveDataDir([]string{"app.exe"})
		if got != "" {
			t.Fatalf("相对路径应无返回值, 得到 %q", got)
		}
		if !errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("期望 ErrDataDirNotAbsolute, 得到: %v", err)
		}
	})

	t.Run("LOCALAPPDATA为空串", func(t *testing.T) {
		setResolverEnv(t, "", "", "")
		_, err := resolveDataDir([]string{"app.exe"})
		if !errors.Is(err, ErrLocalAppDataUndefined) {
			t.Fatalf("期望 ErrLocalAppDataUndefined, 得到: %v", err)
		}
	})

	t.Run("LOCALAPPDATA真未定义", func(t *testing.T) {
		orig, had := os.LookupEnv("LOCALAPPDATA")
		if err := os.Unsetenv("LOCALAPPDATA"); err != nil {
			t.Fatalf("unset LOCALAPPDATA 失败: %v", err)
		}
		t.Cleanup(func() {
			if had {
				if err := os.Setenv("LOCALAPPDATA", orig); err != nil {
					t.Errorf("恢复 LOCALAPPDATA 失败: %v", err)
				}
			}
		})
		t.Setenv("APP_ENV", "")
		t.Setenv("LS_DATA_DIR", "")
		_, err := resolveDataDir([]string{"app.exe"})
		if !errors.Is(err, ErrLocalAppDataUndefined) {
			t.Fatalf("期望 ErrLocalAppDataUndefined, 得到: %v", err)
		}
	})

	t.Run("创建失败错误传播", func(t *testing.T) {
		base := t.TempDir()
		setResolverEnv(t, "", "", base)
		// 用普通文件占住路径中段，MkdirAll 必失败（磁盘满/权限类失败同通道传播）
		blocker := filepath.Join(base, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("准备占位文件失败: %v", err)
		}
		target := filepath.Join(blocker, "sub")
		got, err := resolveDataDir([]string{"app.exe", "--data-dir", target})
		if got != "" {
			t.Fatalf("创建失败应无返回值, 得到 %q", got)
		}
		if err == nil {
			t.Fatal("创建失败应返回错误")
		}
		if errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("创建失败不应误判为相对路径错误: %v", err)
		}
	})
}

// —— 创建幂等：目录已存在时解析照常成功 ——

func TestResolveDataDirIdempotentCreate(t *testing.T) {
	base := t.TempDir()
	setResolverEnv(t, "", "", base)
	want := filepath.Join(base, dataDirDefaultName)
	if err := os.MkdirAll(want, 0755); err != nil {
		t.Fatalf("预建目录失败: %v", err)
	}
	for range 2 {
		got, err := resolveDataDir([]string{"app.exe"})
		resolveOK(t, got, err, want)
	}
}

// —— 缓存行为：进程内一次解析，首解结果（含失败）固化 ——

func TestDataDirCache(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
		resetDataDirCache()
	})

	t.Run("首解成功后不再感知输入变化", func(t *testing.T) {
		resetDataDirCache()
		base := t.TempDir()
		setResolverEnv(t, "", filepath.Join(base, "env-B"), base)
		argA := filepath.Join(base, "arg-A")
		os.Args = []string{"app.exe", "--data-dir", argA}

		got, err := DataDir()
		resolveOK(t, got, err, argA)

		// 输入全换（参数 + 环境变量），缓存结果应不变
		os.Args = []string{"app.exe", "--data-dir", filepath.Join(base, "arg-C")}
		t.Setenv("LS_DATA_DIR", filepath.Join(base, "env-D"))
		got2, err2 := DataDir()
		if err2 != nil {
			t.Fatalf("二次调用意外失败: %v", err2)
		}
		if got2 != argA {
			t.Fatalf("缓存失效: 二次调用 = %q, 期望仍为 %q", got2, argA)
		}
	})

	t.Run("首解失败同样固化", func(t *testing.T) {
		resetDataDirCache()
		base := t.TempDir()
		setResolverEnv(t, "", "", base)
		os.Args = []string{"app.exe", "--data-dir", "relative"}
		_, err := DataDir()
		if !errors.Is(err, ErrDataDirNotAbsolute) {
			t.Fatalf("首解期望 ErrDataDirNotAbsolute, 得到: %v", err)
		}

		// 修正参数后调用，失败结果应保持固化（调用方应致命退出而非重试）
		os.Args = []string{"app.exe", "--data-dir", filepath.Join(base, "fixed")}
		_, err2 := DataDir()
		if !errors.Is(err2, ErrDataDirNotAbsolute) {
			t.Fatalf("失败应被缓存, 二次调用得到: %v", err2)
		}
	})
}
