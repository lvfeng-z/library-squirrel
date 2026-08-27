import { defineStore } from 'pinia'

/**
 * 接收分享入口 store：深链到达（后端 share-events receive-link 事件 / 启动期待处理链接
 * 拉取）与手动入口（分享管理弹窗 [接收分享]）汇入统一状态，驱动 MainLayout 挂载的
 * ShareReceiveDialog 打开。
 */

export const useShareReceiveStore = defineStore('shareReceive', {
  state: (): {
    /** 接收对话框可见性 */
    visible: boolean
    /** 预填链接（深链到达时带入；空=手动粘贴入口） */
    initialLink: string
  } => {
    return { visible: false, initialLink: '' }
  },
  actions: {
    /**
     * 打开接收分享对话框。
     * @param link 预填链接（空串=手动粘贴）
     */
    openWith(link: string): void {
      this.initialLink = link
      this.visible = true
    }
  }
})
