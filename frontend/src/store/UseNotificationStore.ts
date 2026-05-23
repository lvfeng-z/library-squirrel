import { defineStore } from 'pinia'
import NotificationItem from '@renderer/model/util/NotificationItem.ts'
import { v4 } from 'uuid'
import { ElNotification } from 'element-plus'
import { h } from 'vue'
import { isNullish, notNullish } from '@renderer/utils/CommonUtil.ts'

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
    add(notificationItem: NotificationItem): string {
      const id: string = v4()
      notificationItem.id = id
      this.notificationMap.set(id, notificationItem)
      this.notificationList.push(notificationItem)
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
    remove(
      id: string,
      notificationConfig?: { msg: string; type?: 'error' | 'primary' | 'success' | 'warning' | 'info'; duration?: number }
    ): void {
      this.notificationMap.delete(id)
      const index = this.notificationList.findIndex((notification) => notification.id === id)
      if (index !== -1) {
        this.notificationList.splice(index, 1)
      }
      if (notNullish(notificationConfig)) {
        startNotify({
          msg: notificationConfig.msg,
          type: isNullish(notificationConfig.type) ? 'info' : notificationConfig.type,
          duration: isNullish(notificationConfig.duration) ? 3000 : notificationConfig.duration
        })
      }
    }
  },
  getters: {
    count: (state): number => state.notificationMap.size
  }
})

type NotificationConfig = { msg: string; type: 'error' | 'primary' | 'success' | 'warning' | 'info'; duration: number }

const positon: boolean[] = [true, true]
const notificationBuffer: NotificationConfig[] = []
const startNotify = (config: NotificationConfig) => {
  notificationBuffer.push(config)
  recursiveNotify()
}
const recursiveNotify = async () => {
  const index = positon.findIndex((item) => item)
  if (index === -1) {
    return
  }
  positon[index] = false
  if (index === 1) {
    await new Promise<void>((resolve) => setTimeout(() => resolve(), 300))
  }
  let config: NotificationConfig | undefined = undefined
  let currentLength: number = 0
  if (notificationBuffer.length > 0) {
    config = notificationBuffer[0]
    currentLength = notificationBuffer.length
    notificationBuffer.length = 0
  }
  if (config === undefined) {
    positon[index] = true
    return
  }
  const children = [h('i', {}, config.msg)]
  if (currentLength > 1) {
    children.push(
      h(
        'span',
        {
          style: {
            display: 'inline-block',
            backgroundColor: '#f56c6c',
            color: 'white',
            borderRadius: '10px',
            padding: '0 6px',
            fontSize: '12px',
            fontWeight: 'bold',
            marginLeft: '8px',
            lineHeight: '18px',
            minWidth: '18px',
            textAlign: 'center'
          }
        },
        currentLength + '+'
      )
    )
  }
  ElNotification({
    type: config.type,
    message: h('div', {}, children),
    duration: config.duration,
    offset: 80,
    onClose: () => {
      positon[index] = true
      if (notificationBuffer.length > 0) {
        recursiveNotify()
      }
    }
  })
}
