package authorInfo

// 手动拉取面冲突显选粘性记忆的读写锚定：读点在冲突检测分支内（命中直填 resolved 且不记
// 冲突、记忆值映射回条目级两键、记住的候选不在场照常冲突、已显选站点不查询），写点在该
// 站点拉取成功后（勾选才写、未勾选或失败不写、键值编码与读点同构），自动触发面零查询
// 零写入。记忆以记录式替身注入，断言读写调用的键值与次数。

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/settings"
	"github.com/library-squirrel/backend/stickymemory"
)

// memoryCall 一次记忆读/写调用的三元组（Recall 的 value 恒为空串）
type memoryCall struct {
	domain     string
	contextKey string
	value      string
}

// recordingMemory 粘性记忆替身：按 contextKey 存取并记录全部 Remember/Recall 调用（本包
// 单测只消费单一记忆域，命中判定不含 domain，domain 仅随调用记录供断言）
type recordingMemory struct {
	mu        sync.Mutex
	rows      map[string]string
	remembers []memoryCall
	recalls   []memoryCall
}

func newRecordingMemory(seeded map[string]string) *recordingMemory {
	if seeded == nil {
		seeded = map[string]string{}
	}
	return &recordingMemory{rows: seeded}
}

func (m *recordingMemory) Remember(_ context.Context, domain, contextKey, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[contextKey] = value
	m.remembers = append(m.remembers, memoryCall{domain: domain, contextKey: contextKey, value: value})
	return nil
}

func (m *recordingMemory) Recall(_ context.Context, domain, contextKey string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recalls = append(m.recalls, memoryCall{domain: domain, contextKey: contextKey})
	value, ok := m.rows[contextKey]
	return value, ok
}

// seed 预置一条记忆行（读取点命中态构造，不记为调用）
func (m *recordingMemory) seed(contextKey, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[contextKey] = value
}

func (m *recordingMemory) rememberCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.remembers)
}

func (m *recordingMemory) lastRemember() memoryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.remembers[len(m.remembers)-1]
}

func (m *recordingMemory) recallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.recalls)
}

func (m *recordingMemory) lastRecall() memoryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recalls[len(m.recalls)-1]
}

// TestManualFetchRememberedChoiceRoutesWithoutConflict 记忆命中直路由：同站点双候选未显选，
// 冲突分支查记忆命中（值为候选全键）→ 无冲突载荷、显选按记忆值补全为条目级两键下达能力桥、
// 元数据照常回写；记忆直填不带勾选态，本次不重写记忆行
func TestManualFetchRememberedChoiceRoutesWithoutConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.candidatesBySite["pixiv"] = multiCandidatePlugins()
	env.fetcher.plan["12345"] = fetchScript{meta: metaOf("记忆直路由后的新名", "", "", "", "")}
	env.memory.seed(
		siteFetchMemoryContextKey("pixiv", multiCandidatePlugins()),
		stickymemory.CandidateFullKey("p-b", "b-main"))

	outcome, err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID(), nil)
	if err != nil {
		t.Fatalf("记忆命中应免问直路由, 实际报错: %v", err)
	}
	if len(outcome.Conflicts) != 0 {
		t.Fatalf("记忆命中不应返回冲突载荷, 实际 %+v", outcome.Conflicts)
	}
	if got := env.fetcher.lastChosenKey(); got != "p-b/b-main" {
		t.Fatalf("记忆值应补全为条目级两键下达能力桥, 实际 %q", got)
	}
	if got := env.loadSiteAuthor(t, row.GetID()).AuthorName.String; got != "记忆直路由后的新名" {
		t.Fatalf("记忆直路由应照常回写元数据, 实际 %v", got)
	}
	if got := env.memory.recallCount(); got != 1 {
		t.Fatalf("冲突分支应恰好查询一次记忆, 实际 %d 次", got)
	}
	recall := env.memory.lastRecall()
	if recall.domain != entity.DomainSiteAuthorFetchDisambiguation {
		t.Fatalf("记忆查询域应为作者拉取消歧域, 实际 %q", recall.domain)
	}
	siteDomain, candidateKeys := stickymemory.ParseContextKey(recall.contextKey)
	if siteDomain != "pixiv" || !reflect.DeepEqual(candidateKeys, []string{
		stickymemory.CandidateFullKey("p-a", "a-main"),
		stickymemory.CandidateFullKey("p-b", "b-main"),
	}) {
		t.Fatalf("记忆查询键应为站点键与候选全键组合（按字典序）, 实际 %q / %v", siteDomain, candidateKeys)
	}
	if got := env.memory.rememberCount(); got != 0 {
		t.Fatalf("记忆直填不带勾选态, 不应重写记忆行, 实际 %d 次", got)
	}
}

// TestManualFetchRememberedCandidateAbsentStillConflicts 记住的候选不在当前候选集：命中行
// 的值指向不在场候选（插件停用/卸载后残留态）→ 视为未命中，照常记冲突且不调用任何插件
func TestManualFetchRememberedCandidateAbsentStillConflicts(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.candidatesBySite["pixiv"] = multiCandidatePlugins()
	env.fetcher.plan["12345"] = fetchScript{meta: metaOf("不应到达", "", "", "", "")}
	env.memory.seed(
		siteFetchMemoryContextKey("pixiv", multiCandidatePlugins()),
		stickymemory.CandidateFullKey("p-c", "c-main"))

	outcome, err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID(), nil)
	if err != nil {
		t.Fatalf("候选不在场应回落冲突询问而非报错: %v", err)
	}
	if len(outcome.Conflicts) != 1 || outcome.Conflicts[0].SiteKey != "pixiv" {
		t.Fatalf("记住的候选不在场应照常记冲突, 实际 %+v", outcome.Conflicts)
	}
	if got := env.fetcher.callCount(); got != 0 {
		t.Fatalf("冲突路径不应调用插件, 实际 %d 次", got)
	}
	if got := env.memory.rememberCount(); got != 0 {
		t.Fatalf("冲突回落不应写记忆, 实际 %d 次", got)
	}
}

// TestAutoFetchNeverTouchesDisambiguationMemory 自动触发面（作品入库后通知链）不触达记忆：
// 站点候选成冲突态仍直接拉取（自动面不做冲突检测），全程零记忆查询、零记忆写入
func TestAutoFetchNeverTouchesDisambiguationMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	if err := env.settings.SaveSettings([]settings.SettingChange{{Path: "authorSettings.autoFetchInfo", Value: true}}); err != nil {
		t.Fatalf("开启自动拉取开关失败: %v", err)
	}
	// 双候选（若自动面误走冲突检测则会前置收口而非拉取）
	env.fetcher.candidatesBySite["pixiv"] = multiCandidatePlugins()
	env.fetcher.plan["12345"] = fetchScript{meta: metaOf("自动名", "", "", "", "")}

	env.svc.OnSiteAuthorsUpserted([]int64{row.GetID()})
	deadline := time.Now().Add(2 * time.Second)
	for env.fetcher.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := env.fetcher.callCount(); got != 1 {
		t.Fatalf("自动触发面应直接拉取一次, 实际 %d 次", got)
	}
	if got := env.memory.recallCount(); got != 0 {
		t.Fatalf("自动触发面不应查询记忆, 实际 %d 次", got)
	}
	if got := env.memory.rememberCount(); got != 0 {
		t.Fatalf("自动触发面不应写记忆, 实际 %d 次", got)
	}
}

// TestManualFetchRememberWrittenOnSuccessWithEncodedKey 显选带勾选且拉取成功后落记忆：
// 域=作者拉取消歧域、值=显选候选全键、上下文键=站点键与候选组合（ParseContextKey 反解
// 断言）；条目键缺省的显选经补全后同样落表（值含补全条目 id）；已显选站点不查记忆
func TestManualFetchRememberWrittenOnSuccessWithEncodedKey(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	row := env.seedSiteAuthor(t, "pixiv", "12345", nil)
	env.fetcher.candidatesBySite["pixiv"] = multiCandidatePlugins()
	env.fetcher.plan["12345"] = fetchScript{meta: metaOf("记住后的新名", "", "", "", "")}

	// 两键显式 + 勾选：成功后写一条
	chosen := []*dto.SiteAuthorFetchChoice{{
		SiteKey: "pixiv", PluginPublicId: "p-b", ExtensionId: "b-main", Remember: true,
	}}
	if _, err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID(), chosen); err != nil {
		t.Fatalf("显选拉取失败: %v", err)
	}
	if got := env.memory.rememberCount(); got != 1 {
		t.Fatalf("勾选且成功应写一条记忆, 实际 %d 条", got)
	}
	write := env.memory.lastRemember()
	if write.domain != entity.DomainSiteAuthorFetchDisambiguation {
		t.Fatalf("写入域应为作者拉取消歧域, 实际 %q", write.domain)
	}
	if write.value != stickymemory.CandidateFullKey("p-b", "b-main") {
		t.Fatalf("写入值应为显选候选全键, 实际 %q", write.value)
	}
	if pluginId, extId := stickymemory.SplitCandidateFullKey(write.value); pluginId != "p-b" || extId != "b-main" {
		t.Fatalf("写入值应反解为两键, 实际 (%q, %q)", pluginId, extId)
	}
	siteDomain, candidateKeys := stickymemory.ParseContextKey(write.contextKey)
	if siteDomain != "pixiv" || !reflect.DeepEqual(candidateKeys, []string{
		stickymemory.CandidateFullKey("p-a", "a-main"),
		stickymemory.CandidateFullKey("p-b", "b-main"),
	}) {
		t.Fatalf("写入键应为站点键与候选全键组合（按字典序，与读点同构）, 实际 %q / %v", siteDomain, candidateKeys)
	}
	if got := env.memory.recallCount(); got != 0 {
		t.Fatalf("已显选站点不应查记忆（读点在冲突分支内）, 实际 %d 次", got)
	}

	// 条目键缺省 + 勾选：补全路径保留勾选态，写入值含补全后的条目 id
	defaulted := []*dto.SiteAuthorFetchChoice{{
		SiteKey: "pixiv", PluginPublicId: "p-a", Remember: true,
	}}
	if _, err := env.svc.FetchSiteAuthorInfoById(context.Background(), row.GetID(), defaulted); err != nil {
		t.Fatalf("条目键缺省的显选拉取失败: %v", err)
	}
	if got := env.memory.rememberCount(); got != 2 {
		t.Fatalf("补全路径的勾选态应随显选保留并落表, 实际 %d 条", got)
	}
	if got := env.memory.lastRemember().value; got != stickymemory.CandidateFullKey("p-a", "a-main") {
		t.Fatalf("写入值应含补全后的条目 id, 实际 %q", got)
	}
}

// TestManualFetchRememberSkippedWhenUncheckedOrFailed 未勾选或拉取失败不写记忆：勾选态与
// 成功态须同时成立才落表（失败的选择不入记忆，用户下次仍会被问、可改选）
func TestManualFetchRememberSkippedWhenUncheckedOrFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("内存 SQLite 依赖 CGO")
	}
	env := newFetchTestEnv(t)
	okRow := env.seedSiteAuthor(t, "pixiv", "111", nil)
	badRow := env.seedSiteAuthor(t, "pixiv", "222", nil)
	env.fetcher.candidatesBySite["pixiv"] = multiCandidatePlugins()
	env.fetcher.plan["111"] = fetchScript{meta: metaOf("成功名", "", "", "", "")}
	env.fetcher.plan["222"] = fetchScript{
		meta:         metaOf("失败名", "", "", "", ""),
		errAfterMeta: errors.New("注入失败：meta 之后流中断"),
	}

	// 成功但未勾选：不写
	unchecked := []*dto.SiteAuthorFetchChoice{{
		SiteKey: "pixiv", PluginPublicId: "p-b", ExtensionId: "b-main",
	}}
	if _, err := env.svc.FetchSiteAuthorInfoById(context.Background(), okRow.GetID(), unchecked); err != nil {
		t.Fatalf("未勾选显选拉取失败: %v", err)
	}
	if got := env.memory.rememberCount(); got != 0 {
		t.Fatalf("未勾选不应写记忆, 实际 %d 条", got)
	}

	// 勾选但失败：不写
	checked := []*dto.SiteAuthorFetchChoice{{
		SiteKey: "pixiv", PluginPublicId: "p-b", ExtensionId: "b-main", Remember: true,
	}}
	if _, err := env.svc.FetchSiteAuthorInfoById(context.Background(), badRow.GetID(), checked); err == nil {
		t.Fatal("注入失败的拉取应上抛")
	}
	if got := env.memory.rememberCount(); got != 0 {
		t.Fatalf("拉取失败不应写记忆, 实际 %d 条", got)
	}
}
