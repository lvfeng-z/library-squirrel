import { defineStore } from 'pinia'
import { v4 } from 'uuid'
import { isNullish } from '@renderer/utils/CommonUtil.ts'
import {
  type NotificationItem,
  type NewNotificationItem
} from '@renderer/model/util/NotificationItem.ts'

export const useNotificationStore = defineStore('notification', {
  state: (): {
    notificationMap: Map<string, NotificationItem>
    notificationList: NotificationItem[]
  } => {
    return {
      notificationMap: new Map<string, NotificationItem>(),
      notificationList: []
    }
  },
  actions: {
    /**
     * 新增通知。store 分配 id/createTime 并存入副本，返回 id 供外部持有以做后续 update/remove。
     */
    add(item: NewNotificationItem): string {
      const id: string = v4()
      const stored: NotificationItem = { ...item, id, createTime: Date.now() }
      this.notificationMap.set(id, stored)
      // 最新通知插在列表头部：列表即展示序（最新在前），分页第 1 页取到最新条目
      this.notificationList.unshift(stored)
      return id
    },
    get(id: string): NotificationItem | undefined {
      return this.notificationMap.get(id)
    },
    getRange(startIndex: number, endIndex: number): NotificationItem[] {
      const listLength = this.notificationList.length
      // 处理边界情况
      if (listLength === 0) {
        return []
      }
      // 确保 startIndex 在有效范围内
      if (startIndex < 0) {
        startIndex = 0
      } else if (startIndex >= listLength) {
        return []
      }
      // 如果没有提供 endIndex，则获取从 startIndex 到末尾的所有数据
      if (endIndex === undefined) {
        return this.notificationList.slice(startIndex)
      }
      // 确保 endIndex 在有效范围内
      if (endIndex < 0) {
        return []
      } else if (endIndex >= listLength) {
        endIndex = listLength - 1
      }
      // 如果起始索引大于结束索引，交换它们
      if (startIndex > endIndex) {
        ;[startIndex, endIndex] = [endIndex, startIndex]
      }
      // 使用 slice 方法获取区间数据 (包含 startIndex，不包含 endIndex + 1)
      return this.notificationList.slice(startIndex, endIndex + 1)
    },
    /**
     * 原地合并更新通知（progress 等顶层字段整体替换，非深合并）。终态置 terminal:true 后，
     * 调用方应放弃持有的 id 引用（如任务 store 清空 notificationId），使该通知脱离全量清理路径。
     */
    update(id: string, partial: Partial<NewNotificationItem>): void {
      const stored = this.notificationMap.get(id)
      if (isNullish(stored)) {
        return
      }
      Object.assign(stored, partial)
    },
    remove(id: string): void {
      this.notificationMap.delete(id)
      const index = this.notificationList.findIndex((notification) => notification.id === id)
      if (index !== -1) {
        this.notificationList.splice(index, 1)
      }
    }
  },
  getters: {
    /** 全部通知数（分页 total 用） */
    count: (state): number => state.notificationMap.size,
    /** 非终态（活跃）通知数（角标用） */
    activeCount: (state): number => state.notificationList.filter((notification) => !notification.terminal).length
  }
})
