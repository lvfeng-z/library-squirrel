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

// siteAuthorFetcher 站点作者信息拉取能力实现：能力广播路由——候选为声明 siteAuthorFetch 能力包、
// 且有条目把本次请求站点键列入声明站点的已激活插件条目（候选粒度 = 插件 × 条目，候选集内条目
// 身份唯一），经 route 基座按序逐个调用 FetchSiteAuthorInfo 流（请求携带候选条目 id，插件侧按
// 条目分派到对应实现），首个成功者即终点、任一候选失败即终止并点名候选；候选为空时收口报
// 「无已激活插件声明该能力覆盖该站点」。交互面显选键（插件公开 ID + 条目 id 两键）经候选排序键
// 置于首位。实现 authorInfo.SiteAuthorFetcher 接口（authorInfo 模块定义、plugin 实现——
// ORCHESTRATION_BY_CALLER）。声明驱动：插件未声明能力包、或无条目把该站点键列入声明时不成候选，
// 不盲调 gRPC
type siteAuthorFetcher struct {
	activeLister ActivePluginLister
	entryQuery   SiteAuthorFetchEntryQuerier
	accessor     ServiceAccessor
}

// NewSiteAuthorFetcher 创建站点作者信息拉取器。activeLister 枚举已激活插件供广播遍历，
// entryQuery 供按条目枚举声明（插件未声明能力包、或无条目把请求站点键列入声明则不调用），
// accessor 取插件 gRPC 服务客户端
func NewSiteAuthorFetcher(activeLister ActivePluginLister, entryQuery SiteAuthorFetchEntryQuerier,
	accessor ServiceAccessor) *siteAuthorFetcher {
	return &siteAuthorFetcher{activeLister: activeLister, entryQuery: entryQuery, accessor: accessor}
}

// authorFetchCandidate 作者拉取的候选路由原子：插件 × siteAuthorFetch 条目
type authorFetchCandidate struct {
	plugin        ActivePlugin
	extensionId   string // 条目 id（拉取请求携带的实例身份）
	extensionName string // 条目显示名（候选展示名第二段）
}

// authorFetchOrderKey 候选全键：插件公开 ID 与条目 id 以 NUL 拼接。NUL 不在标识符取值域内，
// ("a","bc") 与 ("ab","c") 不会撞键。与任务创建面的候选排序键同构
func authorFetchOrderKey(c authorFetchCandidate) string {
	return c.plugin.PublicID + "\x00" + c.extensionId
}

// FetchSiteAuthorInfo 拉取作者最新元数据与头像资源：候选为声明 siteAuthorFetch 能力包且有条目
// 声明了该站点键的已激活插件条目，按候选全键字典序逐个尝试，首个成功者即终点、任一候选失败即
// 终止并点名候选。显选两键（chosenPluginPublicId + chosenExtensionId）联合命中候选集时该候选置于
// 候选序首位（交互面显选），两键均空时候选全按全键字典序（自动面与单候选场景即此态）。
// 首块 meta 交 onMeta（返回是否继续接收头像字节——false 时取消流并按成功收尾），后续字节块交
// onData；任一回调返回错误即中止流并作为整体失败上抛。ctx 打断承接（CreateWorkInfoWithContext 同模式）
func (f *siteAuthorFetcher) FetchSiteAuthorInfo(ctx context.Context, siteKey, siteAuthorId, chosenPluginPublicId, chosenExtensionId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	adapter := &siteAuthorFetchAdapter{
		fetcher:           f,
		siteKey:           siteKey,
		siteAuthorId:      siteAuthorId,
		onMeta:            onMeta,
		onData:            onData,
		chosenPluginId:    chosenPluginPublicId,
		chosenExtensionId: chosenExtensionId,
	}
	_, err := route.Route[authorFetchCandidate, struct{}](ctx, adapter)
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

// ListSiteAuthorFetchCandidates 指定站点键下的候选清单（条目级消费面：插件 × siteAuthorFetch
// 条目对，ExtensionId 携带条目 id）：与拉取链共用同一候选枚举（声明驱动 + 站点归属 + 服务客户端
// 可用性），按候选全键（插件公开 ID 与条目 id 的 NUL 拼接）字典序返回，首位即默认选中项。
// 候选展示名为「插件名 · 条目名」两段（插件未设置展示名时回落公开标识）
func (f *siteAuthorFetcher) ListSiteAuthorFetchCandidates(_ context.Context, siteKey string) ([]*dto.PluginCandidate, error) {
	candidates := f.fetchCandidates(siteKey)
	sort.Slice(candidates, func(i, j int) bool { return authorFetchOrderKey(candidates[i]) < authorFetchOrderKey(candidates[j]) })
	result := make([]*dto.PluginCandidate, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, &dto.PluginCandidate{
			PluginPublicId: c.plugin.PublicID,
			PluginName:     candidateDisplayName(c),
			ExtensionId:    c.extensionId,
		})
	}
	return result, nil
}

// candidatePluginName 插件展示名：插件未设置展示名时回落公开标识
func candidatePluginName(plugin ActivePlugin) string {
	if plugin.Name != "" {
		return plugin.Name
	}
	return plugin.PublicID
}

// candidateDisplayName 候选展示名：「插件名 · 条目名」两段拼接（条目显示名由清单声明必填）
func candidateDisplayName(c authorFetchCandidate) string {
	return candidatePluginName(c.plugin) + " · " + c.extensionName
}

// fetchCandidates 候选清单（插件 × 条目对）：已激活插件的 siteAuthorFetch 条目中把该站点键列入
// 声明站点者，且插件有可用服务客户端。候选在发现侧就按客户端可用性过滤——缺客户端的声明者不进
// 候选，不会在调用时才暴露为失败
func (f *siteAuthorFetcher) fetchCandidates(siteKey string) []authorFetchCandidate {
	if f.entryQuery == nil {
		return nil
	}
	var candidates []authorFetchCandidate
	for _, plugin := range f.activeLister.ListActivePlugins() {
		entries := f.entryQuery.SiteAuthorFetchEntries(plugin.PublicID)
		if len(entries) == 0 {
			continue
		}
		if _, ok := f.accessor.GetServices(plugin.PublicID); !ok {
			continue
		}
		for _, entry := range entries {
			if !siteDeclared(entry.Sites, siteKey) {
				continue
			}
			candidates = append(candidates, authorFetchCandidate{
				plugin:        plugin,
				extensionId:   entry.ID,
				extensionName: entry.Name,
			})
		}
	}
	return candidates
}

// siteDeclared 站点键是否在条目声明的站点键清单内
func siteDeclared(declared []string, siteKey string) bool {
	for _, key := range declared {
		if key == siteKey {
			return true
		}
	}
	return false
}

// siteAuthorFetchAdapter 站点作者拉取的 route 接入：发现候选、按显选键给候选排序、
// 把单候选流调用按成功/失败分类
type siteAuthorFetchAdapter struct {
	fetcher      *siteAuthorFetcher
	siteKey      string
	siteAuthorId string
	onMeta       func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error)
	onData       func(data []byte) error
	// chosenPluginId/chosenExtensionId 显选候选键（插件公开 ID + 条目 id）：两键联合命中候选集内
	// 一个候选时该候选排在其他候选之前；两键均空 = 无显选，候选全按全键字典序回落
	chosenPluginId    string
	chosenExtensionId string
}

// Candidates 候选=声明 siteAuthorFetch 能力包且有条目声明了本次站点键、并有可用服务客户端的
// 已激活插件条目。排序由基座按 OrderKey 完成，此处不预排序
func (a *siteAuthorFetchAdapter) Candidates(_ context.Context) ([]authorFetchCandidate, error) {
	return a.fetcher.fetchCandidates(a.siteKey), nil
}

// OrderKey 候选排序键：显选候选前缀 "0"、其余前缀 "1"，后接候选全键——基座按本键字典序排序，
// 显选因此经排序键表达（候选集内全键唯一，键亦唯一；非显选候选间保持全键字典序）
func (a *siteAuthorFetchAdapter) OrderKey(c authorFetchCandidate) string {
	if a.hasChosen() && c.plugin.PublicID == a.chosenPluginId && c.extensionId == a.chosenExtensionId {
		return "0" + authorFetchOrderKey(c)
	}
	return "1" + authorFetchOrderKey(c)
}

// hasChosen 本次拉取是否携带显选键（两键均空 = 未显选）
func (a *siteAuthorFetchAdapter) hasChosen() bool {
	return a.chosenPluginId != "" || a.chosenExtensionId != ""
}

// Describe 候选点名：插件展示名 + 条目 id，定位一个路由原子
func (a *siteAuthorFetchAdapter) Describe(c authorFetchCandidate) string {
	return fmt.Sprintf("%s/%s", candidatePluginName(c.plugin), c.extensionId)
}

// Invoke 调用单候选流：流完整消费为成功，其余错误为失败（终止并携带原因）。拉取请求携带候选
// 条目 id（插件侧按条目分派到对应实现）
func (a *siteAuthorFetchAdapter) Invoke(ctx context.Context, c authorFetchCandidate) (struct{}, route.Outcome, error) {
	services, ok := a.fetcher.accessor.GetServices(c.plugin.PublicID)
	if !ok {
		// 候选由 Candidates 按客户端可用性筛出，此处缺失说明装配态在路由期间变化
		return struct{}{}, route.Failed, errors.New("插件服务客户端不可用")
	}
	if err := consumeAuthorInfoStream(ctx, services.SiteAuthorFetch, a.siteKey, a.siteAuthorId, c.extensionId, a.onMeta, a.onData); err != nil {
		return struct{}{}, route.Failed, err
	}
	return struct{}{}, route.Succeeded, nil
}

// consumeAuthorInfoStream 单候选流消费：开流（请求携带条目 id）→ 首块必为 meta → 按 onMeta
// 决定继续/收尾 → 后续资源块交 onData。meta 块重复、资源块先于 meta、流终结无 meta 均为插件侧
// 协议违规
func consumeAuthorInfoStream(ctx context.Context, client gen.SiteAuthorFetchServiceClient, siteKey, siteAuthorId, extensionId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.FetchSiteAuthorInfo(streamCtx, &gen.FetchSiteAuthorInfoRequest{
		SiteKey:      siteKey,
		SiteAuthorId: siteAuthorId,
		ExtensionId:  extensionId,
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
