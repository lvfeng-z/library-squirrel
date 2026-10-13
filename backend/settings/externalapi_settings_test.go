package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/library-squirrel/backend/base/logger"
)

// errSyncStub 运行时同步失败替身错误
var errSyncStub = errors.New("同步失败替身")

// TestMain 测试环境挂 nop logger（生产 logger 由应用启动时 Init，测试中为 nil）
func TestMain(m *testing.M) {
	if logger.Log == nil {
		logger.Log = zap.NewNop().Sugar()
	}
	os.Exit(m.Run())
}

// fakeExternalApiRuntime 外部接口运行时联动替身：记录同步目标态快照，token 生成返回固定值
type fakeExternalApiRuntime struct {
	syncs    []string
	newToken string
	syncErr  error
}

func (f *fakeExternalApiRuntime) SyncExternalApi(enabled bool, port int, token string) error {
	f.syncs = append(f.syncs, fmt.Sprintf("enabled=%v,port=%d,token=%s", enabled, port, token))
	return f.syncErr
}

func (f *fakeExternalApiRuntime) NewExternalApiToken() (string, error) {
	return f.newToken, nil
}

func (f *fakeExternalApiRuntime) ExternalApiRuntimeStatus() (bool, string, string) {
	return false, "", ""
}

// readSettingsFile 读取 settings.json 中的 externalApi 节
func readSettingsFile(t *testing.T, path string) ExternalApiSettings {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取设置文件失败: %v", err)
	}
	var view struct {
		ExternalApi ExternalApiSettings `json:"externalApi"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("设置文件不是合法 JSON: %v", err)
	}
	return view.ExternalApi
}

// newTestService 在临时目录构建设置服务与 Handler（externalApi 联动用替身）
func newTestService(t *testing.T) (*Handler, *fakeExternalApiRuntime, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	svc := NewService(path)
	fake := &fakeExternalApiRuntime{newToken: "tok-regen"}
	handler := NewHandler(svc, fake)
	return handler, fake, path
}

// TestExternalApiSettingsDefaultOff 外部接口设置默认关：NewSettings 与文件缺失/缺节的
// 加载路径均回落默认值（enabled=false、port=0、token 空）
func TestExternalApiSettingsDefaultOff(t *testing.T) {
	if NewSettings().ExternalApi.Enabled {
		t.Fatal("NewSettings 外部接口期望默认关")
	}

	// 文件缺失：koanf 默认层生效
	svc := NewService(filepath.Join(t.TempDir(), "none", "settings.json"))
	if s := svc.GetSettings().ExternalApi; s.Enabled || s.Port != 0 || s.Token != "" {
		t.Fatalf("文件缺失期望默认值，实际 %+v", s)
	}

	// 文件存在但无 externalApi 节：字段缺失回落代码默认值
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"workdir":"D:/lib"}`), 0644); err != nil {
		t.Fatalf("写入设置文件失败: %v", err)
	}
	svc = NewService(path)
	if s := svc.GetSettings().ExternalApi; s.Enabled {
		t.Fatalf("缺节期望默认关，实际 %+v", s)
	}
}

// TestSetExternalApiEnabledPersistsAndSyncs 开关切换：持久化到设置文件，开启前空 token
// 自动生成持久化，并携带最新目标态同步运行时
func TestSetExternalApiEnabledPersistsAndSyncs(t *testing.T) {
	handler, fake, path := newTestService(t)

	resp := handler.SetExternalApiEnabled(true)
	if !resp.Success {
		t.Fatalf("开启外部接口期望成功信封，实际失败: %s", resp.Msg)
	}
	saved := readSettingsFile(t, path)
	if !saved.Enabled {
		t.Fatalf("设置文件期望 externalApi.enabled=true，实际 %+v", saved)
	}
	if saved.Token != "tok-regen" {
		t.Fatalf("空 token 期望开启前自动生成持久化，实际 %q", saved.Token)
	}
	// 末次同步快照携带最终目标态（开、随机端口、生成后的 token）
	last := fake.syncs[len(fake.syncs)-1]
	if last != "enabled=true,port=0,token=tok-regen" {
		t.Fatalf("末次同步期望目标态 enabled=true,port=0,token=tok-regen，实际 %q", last)
	}

	resp = handler.SetExternalApiEnabled(false)
	if !resp.Success {
		t.Fatalf("关闭外部接口期望成功信封，实际失败: %s", resp.Msg)
	}
	if saved = readSettingsFile(t, path); saved.Enabled {
		t.Fatal("设置文件期望 externalApi.enabled=false")
	}
}

// TestResetExternalApiTokenReplacesStoredToken token 重置换新：设置文件中 token 换为
// 新值，响应携带新 token，运行时同步携带新 token
func TestResetExternalApiTokenReplacesStoredToken(t *testing.T) {
	handler, fake, path := newTestService(t)
	// 预置：开启（token 自动生成 tok-regen）
	if resp := handler.SetExternalApiEnabled(true); !resp.Success {
		t.Fatalf("预置开启失败: %s", resp.Msg)
	}

	fake.newToken = "tok-brand-new"
	resp := handler.ResetExternalApiToken()
	if !resp.Success {
		t.Fatalf("重置 token 期望成功信封，实际失败: %s", resp.Msg)
	}
	if resp.Data != "tok-brand-new" {
		t.Fatalf("响应期望携带新 token，实际 %q", resp.Data)
	}
	if saved := readSettingsFile(t, path); saved.Token != "tok-brand-new" {
		t.Fatalf("设置文件期望 token 换新，实际 %q", saved.Token)
	}
	last := fake.syncs[len(fake.syncs)-1]
	if last != "enabled=true,port=0,token=tok-brand-new" {
		t.Fatalf("末次同步期望携带新 token，实际 %q", last)
	}
}

// TestResetRegeneratesExternalApiToken 整体重置把 token 清回空默认后自动重新生成持久化
// （恒非空不变量），开关回落默认关
func TestResetRegeneratesExternalApiToken(t *testing.T) {
	handler, _, path := newTestService(t)
	if resp := handler.SetExternalApiEnabled(true); !resp.Success {
		t.Fatalf("预置开启失败: %s", resp.Msg)
	}

	if resp := handler.Reset(); !resp.Success {
		t.Fatalf("重置设置期望成功信封，实际失败: %s", resp.Msg)
	}
	saved := readSettingsFile(t, path)
	if saved.Enabled {
		t.Fatal("重置后外部接口期望回落默认关")
	}
	if saved.Token == "" {
		t.Fatal("重置后 token 期望重新生成非空值（恒非空不变量）")
	}
}

// TestSetExternalApiEnabledSyncFailureDegradesWithNote 运行时同步失败为降级：设置已保存
// （成功信封），失败原因进附注
func TestSetExternalApiEnabledSyncFailureDegradesWithNote(t *testing.T) {
	handler, fake, path := newTestService(t)
	fake.syncErr = errSyncStub

	resp := handler.SetExternalApiEnabled(true)
	if !resp.Success {
		t.Fatalf("同步失败期望降级为成功信封（设置已保存），实际失败: %s", resp.Msg)
	}
	if resp.Msg == "" || resp.Msg == "success" {
		t.Fatalf("降级期望附注说明，实际 %q", resp.Msg)
	}
	if saved := readSettingsFile(t, path); !saved.Enabled {
		t.Fatal("降级不回滚设置：文件期望 enabled=true")
	}
}
