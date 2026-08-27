import { defineStore } from 'pinia'
import type { ShareSessionDTO } from '@bindings/github.com/library-squirrel/backend/share/models'
import { ShareSessionDTO as ShareSessionDTOClass } from '@bindings/github.com/library-squirrel/backend/share/models'
import { shareSessions } from '@renderer/apis/http/wrappers/share'
import { useMenuBadgeStore } from '@renderer/store/UseMenuBadgeStore'

/** 「分享」菜单项 slotId（useBuiltinMenus 的 builtin-share 项）——活跃会话红点的注册键 */
export const SHARE_MENU_SLOT_ID = 'builtin-share'

/**
 * 分享会话 store：后端 share-events 事件（progress/complete/state）驱动 + ShareSessions
 * 快照兜底，维护「分享进行中」的一等状态（会话列表/链接/终态）。
 *
 * 会话为进程内运行态（connecting/online/reconnecting/终态）；跨启动的分享账本在
 * share_record 表（分享视图历史区，经 ShareRecords 查询），本 store 不承载。
 * 活跃（在线/重连中）会话数同步写入菜单红点注册表（「分享」菜单项徽标）。
 */

/** 一次发布的过程态（progress/complete 事件驱动；发布完成后转为会话态） */
export interface SharePublishingState {
  shareId: string
  /** collecting=收集中 / registering=注册中继中 */
  phase: 'collecting' | 'registering'
  /** 发布终态：success=已在线 / failed=失败（含取消） */
  status: 'running' | 'success' | 'failed'
  /** 成功时的完整分享链接（含 fragment 密钥） */
  link?: string
  /** 失败原因（取消时为「已取消」） */
  errMsg?: string
}

export const useShareStore = defineStore('share', {
  state: (): {
    /** 进行中/刚完成的发布（shareId → 过程态；保留至弹窗关闭或下次发布） */
    publishings: Record<string, SharePublishingState>
    /** 会话快照（shareId → DTO；state 事件与 ShareSessions 查询共同维护） */
    sessions: Record<string, ShareSessionDTO>
    /** 列表加载中 */
    loading: boolean
  } => {
    return { publishings: {}, sessions: {}, loading: false }
  },
  getters: {
    /** 全部会话（按创建时间升序，展示稳定） */
    sessionList(state): ShareSessionDTO[] {
      return Object.values(state.sessions).sort((a, b) => a.createdAt - b.createdAt)
    },
    /** 在线/重连中的会话数（「分享」菜单红点值） */
    activeSessionCount(state): number {
      return Object.values(state.sessions).filter((s) => s.state === 'online' || s.state === 'reconnecting').length
    }
  },
  actions: {
    /**
     * 处理一条 share-events 事件（{type,data} 由 MainIpcListener 解出后传入）。
     * complete 视为发布终态：忽略其后迟到的 progress。
     */
    onShareEvent(type: string, data: any): void {
      if (type === 'progress') {
        const shareId = data.shareId as string
        const existing = this.publishings[shareId]
        if (existing && existing.status !== 'running') return
        this.publishings[shareId] = {
          shareId,
          phase: data.phase as 'collecting' | 'registering',
          status: 'running'
        }
      } else if (type === 'complete') {
        const shareId = data.shareId as string
        const success = data.success as boolean
        this.publishings[shareId] = {
          shareId,
          phase: 'registering',
          status: success ? 'success' : 'failed',
          link: data.link as string | undefined,
          errMsg: success ? undefined : (data.errMsg as string)
        }
        if (success && data.session) {
          this.upsertSession(data.session)
        }
      } else if (type === 'state') {
        this.upsertSession(data)
      }
    },
    /** 前端发起发布时立即标记 running（首个 progress 事件到达前的即时反馈） */
    markPublishStarted(shareId: string): void {
      const existing = this.publishings[shareId]
      if (existing && existing.status !== 'running') return
      this.publishings[shareId] = { shareId, phase: 'collecting', status: 'running' }
    },
    /** 发布启动失败回退（前置校验不过时清除 running 标记） */
    clearPublishing(shareId: string): void {
      delete this.publishings[shareId]
    },
    /** 拉取全量会话快照（启动引导/视图刷新时调用） */
    async loadSessions(): Promise<void> {
      this.loading = true
      try {
        const list = await shareSessions()
        for (const dto of list) {
          this.upsertSession(dto)
        }
      } finally {
        this.loading = false
        this.syncMenuBadge()
      }
    },
    /** 写入/更新会话快照（state 事件与快照查询共用入口；终态不被旧事件回退） */
    upsertSession(data: any): void {
      if (!data?.shareId) return
      const dto = data instanceof ShareSessionDTOClass ? data : ShareSessionDTOClass.createFrom(data)
      const existing = this.sessions[dto.shareId]
      // 终态不可逆：已终态的会话忽略迟到事件（防 revoked → online 回跳）
      if (existing && isTerminalState(existing.state) && !isTerminalState(dto.state)) return
      this.sessions[dto.shareId] = dto
      this.syncMenuBadge()
    },
    /** 活跃会话数写入「分享」菜单红点（消费侧 DynamicSideMenu 经注册表解耦，不直连本 store） */
    syncMenuBadge(): void {
      useMenuBadgeStore().setBadge(SHARE_MENU_SLOT_ID, this.activeSessionCount)
    }
  }
})

/** 会话终态判定（与后端状态机一致：revoked/expired/failed 不可逆） */
function isTerminalState(state: string): boolean {
  return state === 'revoked' || state === 'expired' || state === 'failed'
}
