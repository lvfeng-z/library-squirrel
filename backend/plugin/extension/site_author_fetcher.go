package extension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/route"
	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
)

// siteAuthorFetcher 站点作者信息拉取能力实现：能力广播路由——候选为声明 siteAuthorFetch
// 能力包、且该能力包声明的站点键含本次请求站点键的已激活插件（插件级消费面，候选粒度=插件），
// 经 route 基座按序逐个调用 FetchSiteAuthorInfo 流，首个成功者即终点、任一候选失败即终止并
// 点名插件；候选为空时收口报「无已激活插件声明该能力覆盖该站点」。交互面显选键经候选排序键置于
// 首位。实现 authorInfo.SiteAuthorFetcher 接口（authorInfo 模块定义、plugin 实现——
// ORCHESTRATION_BY_CALLER）。声明驱动：插件未声明能力包或未把该站点列入声明时不成候选，不盲调 gRPC
type siteAuthorFetcher struct {
	activeLister ActivePluginLister
	capsQuery    CapabilityQuerier
	scopeQuery   SiteAuthorFetchScopeQuerier
	accessor     ServiceAccessor
}

// NewSiteAuthorFetcher 创建站点作者信息拉取器。activeLister 枚举已激活插件供广播遍历，
// capsQuery 与 scopeQuery 用于声明驱动（插件未声明能力包、或未把请求站点键列入声明则不调用），
// accessor 取插件 gRPC 服务客户端
func NewSiteAuthorFetcher(activeLister ActivePluginLister, capsQuery CapabilityQuerier,
	scopeQuery SiteAuthorFetchScopeQuerier, accessor ServiceAccessor) *siteAuthorFetcher {
	return &siteAuthorFetcher{activeLister: activeLister, capsQuery: capsQuery, scopeQuery: scopeQuery, accessor: accessor}
}

// FetchSiteAuthorInfo 拉取作者最新元数据与头像资源：候选为声明 siteAuthorFetch 能力包且声明了
// 该站点键的已激活插件，按候选排序键逐个尝试，首个成功者即终点、任一候选失败即终止并点名插件。
// chosenPluginPublicId 非空时该插件候选置于候选序首位（交互面显选），空串时候选全按插件标识字典序。
// 首块 meta 交 onMeta（返回是否继续接收头像字节——false 时取消流并按成功收尾），后续字节块交
// onData；任一回调返回错误即中止流并作为整体失败上抛。ctx 打断承接（CreateWorkInfoWithContext 同模式）
func (f *siteAuthorFetcher) FetchSiteAuthorInfo(ctx context.Context, siteKey, siteAuthorId, chosenPluginPublicId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	adapter := &siteAuthorFetchAdapter{
		fetcher:           f,
		siteKey:           siteKey,
		siteAuthorId:      siteAuthorId,
		onMeta:            onMeta,
		onData:            onData,
		preferredPluginId: chosenPluginPublicId,
	}
	_, err := route.Route[string, struct{}](ctx, adapter)
	if err == nil {
		return nil
	}
	if errors.Is(err, route.ErrNoCandidates) {
		// 零候选：无已激活插件声明该能力包覆盖该站点
		return fmt.Errorf("站点 %s 的作者信息拉取无归属插件：无已激活插件声明 %s 能力覆盖该站点（%w）",
			siteKey, CapabilitySiteAuthorFetch, err)
	}
	return fmt.Errorf("站点 %s 的作者 %s 信息拉取失败：%w", siteKey, siteAuthorId, err)
}

// ListSiteAuthorFetchCandidates 指定站点键下的候选清单（插件级消费面，ExtensionId 恒空串）：
// 与拉取链共用同一候选枚举（声明驱动 + 站点归属 + 服务客户端可用性），按插件标识字典序返回，
// 首位即默认选中项。PluginName 取插件展示名，插件未设置名时回落公开标识
func (f *siteAuthorFetcher) ListSiteAuthorFetchCandidates(ctx context.Context, siteKey string) ([]*dto.PluginCandidate, error) {
	plugins := f.candidatePlugins(ctx, siteKey)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].PublicID < plugins[j].PublicID })
	candidates := make([]*dto.PluginCandidate, 0, len(plugins))
	for _, plugin := range plugins {
		candidates = append(candidates, &dto.PluginCandidate{
			PluginPublicId: plugin.PublicID,
			PluginName:     candidatePluginName(plugin),
		})
	}
	return candidates, nil
}

// candidatePluginName 候选展示名：插件未设置展示名时回落公开标识
func candidatePluginName(plugin ActivePlugin) string {
	if plugin.Name != "" {
		return plugin.Name
	}
	return plugin.PublicID
}

// candidatePlugins 候选插件清单（声明 siteAuthorFetch 能力包、该能力包声明的站点键含 siteKey、
// 且有可用服务客户端的已激活插件）。候选在发现侧就按客户端可用性过滤——缺客户端的声明者不进候选，
// 不会在调用时才暴露为失败
func (f *siteAuthorFetcher) candidatePlugins(_ context.Context, siteKey string) []ActivePlugin {
	var candidates []ActivePlugin
	for _, plugin := range f.activeLister.ListActivePlugins() {
		if !f.pluginDeclares(plugin.PublicID, CapabilitySiteAuthorFetch) {
			continue
		}
		if !f.declaresSite(plugin.PublicID, siteKey) {
			continue
		}
		if _, ok := f.accessor.GetServices(plugin.PublicID); !ok {
			continue
		}
		candidates = append(candidates, plugin)
	}
	return candidates
}

// siteAuthorFetchAdapter 站点作者拉取的 route 接入：发现候选、按置首键给候选排序、
// 把单插件流调用按成功/失败分类
type siteAuthorFetchAdapter struct {
	fetcher      *siteAuthorFetcher
	siteKey      string
	siteAuthorId string
	onMeta       func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error)
	onData       func(data []byte) error
	// preferredPluginId 置首候选键（插件标识）：命中候选集中的插件时该候选排在其他候选之前；
	// 空串=无显选，候选全按标识字典序回落
	preferredPluginId string
}

// Candidates 候选=声明 siteAuthorFetch 能力包且声明了本次站点键、并有可用服务客户端的已激活
// 插件。排序由基座按 OrderKey 完成，此处不预排序
func (a *siteAuthorFetchAdapter) Candidates(ctx context.Context) ([]string, error) {
	plugins := a.fetcher.candidatePlugins(ctx, a.siteKey)
	pluginIds := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		pluginIds = append(pluginIds, plugin.PublicID)
	}
	return pluginIds, nil
}

// OrderKey 候选排序键：置首候选前缀 "0"、其余前缀 "1"，后接插件标识——基座按本键字典序排序，
// 置首因此经排序键表达（候选集内标识唯一，键亦唯一；非置首候选间保持标识字典序）
func (a *siteAuthorFetchAdapter) OrderKey(pluginId string) string {
	if pluginId == a.preferredPluginId {
		return "0" + pluginId
	}
	return "1" + pluginId
}

// Describe 候选点名：插件标识
func (a *siteAuthorFetchAdapter) Describe(pluginId string) string {
	return pluginId
}

// Invoke 调用单插件流：流完整消费为成功，其余错误为失败（终止并携带原因）
func (a *siteAuthorFetchAdapter) Invoke(ctx context.Context, pluginId string) (struct{}, route.Outcome, error) {
	services, ok := a.fetcher.accessor.GetServices(pluginId)
	if !ok {
		// 候选由 Candidates 按客户端可用性筛出，此处缺失说明装配态在路由期间变化
		return struct{}{}, route.Failed, errors.New("插件服务客户端不可用")
	}
	if err := consumeAuthorInfoStream(ctx, services.SiteAuthorFetch, a.siteKey, a.siteAuthorId, a.onMeta, a.onData); err != nil {
		return struct{}{}, route.Failed, err
	}
	return struct{}{}, route.Succeeded, nil
}

// consumeAuthorInfoStream 单插件流消费：开流 → 首块必为 meta → 按 onMeta 决定继续/收尾 →
// 后续资源块交 onData。meta 块重复、资源块先于 meta、流终结无 meta 均为插件侧协议违规
func consumeAuthorInfoStream(ctx context.Context, client gen.SiteAuthorFetchServiceClient, siteKey, siteAuthorId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.FetchSiteAuthorInfo(streamCtx, &gen.FetchSiteAuthorInfoRequest{
		SiteKey:      siteKey,
		SiteAuthorId: siteAuthorId,
	})
	if err != nil {
		return err
	}
	metaSeen := false
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if meta := chunk.GetMeta(); meta != nil {
			if metaSeen {
				return fmt.Errorf("插件作者信息流协议违规：meta 块重复")
			}
			metaSeen = true
			wantAvatar, err := onMeta(meta)
			if err != nil {
				return err
			}
			// 消费侧不需要头像字节：取消流收尾（插件侧阻塞的 send 随流 ctx 中断）
			if !wantAvatar {
				return nil
			}
			continue
		}
		if res := chunk.GetResource(); res != nil {
			if !metaSeen {
				return fmt.Errorf("插件作者信息流协议违规：资源块先于 meta 块")
			}
			if err := onData(res.GetData()); err != nil {
				return err
			}
		}
	}
	if !metaSeen {
		return fmt.Errorf("插件作者信息流协议违规：流缺少 meta 首块")
	}
	return nil
}

// pluginDeclares 查插件是否声明了指定能力；未注入查询器时保守返回 true（向后兼容）
func (f *siteAuthorFetcher) pluginDeclares(pluginPublicId, capability string) bool {
	if f.capsQuery == nil {
		return true
	}
	return hasCapability(f.capsQuery.GetCapabilities(pluginPublicId), capability)
}

// declaresSite 查插件的站点作者拉取能力包是否把该站点键列入声明；未注入查询器时保守返回 true
// （向后兼容）
func (f *siteAuthorFetcher) declaresSite(pluginPublicId, siteKey string) bool {
	if f.scopeQuery == nil {
		return true
	}
	for _, declared := range f.scopeQuery.SiteAuthorFetchSites(pluginPublicId) {
		if declared == siteKey {
			return true
		}
	}
	return false
}
