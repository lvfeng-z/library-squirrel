package extension

import (
	"context"
	"errors"
	"fmt"
	"io"

	pluginsdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/gen"
	pluginsdktransport "github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// siteAuthorFetcher 站点作者信息拉取能力实现：能力广播路由——按字典序遍历声明
// siteAuthorFetch 能力的已激活插件逐个调用 FetchSiteAuthorInfo 流，插件按请求 siteKey
// 归属自判（未归属经 PermissionDenied 跨进程表达，IsSiteNotOwnedStatus 命中即静默跳过），
// 命中一个即止；无归属（无能力插件或全部未归属）返回错误。实现 authorInfo.SiteAuthorFetcher
// 接口（authorInfo 模块定义、plugin 实现——ORCHESTRATION_BY_CALLER）。声明驱动：
// 插件未声明能力时跳过，不盲调 gRPC
type siteAuthorFetcher struct {
	activeLister ActivePluginLister
	capsQuery    CapabilityQuerier
	accessor     ServiceAccessor
}

// NewSiteAuthorFetcher 创建站点作者信息拉取器。activeLister 枚举已激活插件供广播遍历，
// capsQuery 用于声明驱动（插件未声明能力则不调用），accessor 取插件 gRPC 服务客户端
func NewSiteAuthorFetcher(activeLister ActivePluginLister, capsQuery CapabilityQuerier, accessor ServiceAccessor) *siteAuthorFetcher {
	return &siteAuthorFetcher{activeLister: activeLister, capsQuery: capsQuery, accessor: accessor}
}

// FetchSiteAuthorInfo 按站点身份键广播拉取作者最新元数据与头像资源：首块 meta 交 onMeta
// （返回是否继续接收头像字节——false 时取消流并按成功收尾），后续字节块交 onData；
// 任一回调返回错误即中止流并作为整体失败上抛。ctx 打断承接（CreateWorkInfoWithContext 同模式）
func (f *siteAuthorFetcher) FetchSiteAuthorInfo(ctx context.Context, siteKey, siteAuthorId string,
	onMeta func(meta *pluginsdkdto.AuthorInfoMeta) (wantAvatar bool, err error),
	onData func(data []byte) error) error {
	for _, pluginId := range f.activeLister.ListActivePluginIds() {
		if !f.pluginDeclares(pluginId, CapabilitySiteAuthorFetch) {
			continue
		}
		services, ok := f.accessor.GetServices(pluginId)
		if !ok {
			continue
		}
		err := consumeAuthorInfoStream(ctx, services.SiteAuthorFetch, siteKey, siteAuthorId, onMeta, onData)
		if err == nil {
			return nil
		}
		// 未归属信号静默跳过继续广播；其余错误按真失败上抛（命中一个即止）
		if pluginsdktransport.IsSiteNotOwnedStatus(err) {
			continue
		}
		return fmt.Errorf("插件 %s 拉取站点 %s 作者 %s 信息失败: %w", pluginId, siteKey, siteAuthorId, err)
	}
	return fmt.Errorf("站点 %s 的作者信息拉取无归属插件（无已激活插件声明 siteAuthorFetch 能力或全部未归属）", siteKey)
}

// consumeAuthorInfoStream 单插件流消费：开流 → 首块必为 meta → 按 onMeta 决定继续/收尾 →
// 后续资源块交 onData。meta 块重复、资源块先于 meta、流终结无 meta 均为插件侧协议违规。
// 未归属错误（PermissionDenied）可能出现在开流或首个 Recv，原样返回由广播层判别
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
