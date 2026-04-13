import { VNode } from 'vue'

export default class NotificationItem {
  /**
   * 唯一标识
   */
  id: string

  /**
   * 标题
   */
  title: string

  /**
   * 描述
   */
  description: string

  /**
   * 自定义区域渲染函数
   */
  render: (() => VNode) | undefined

  constructor() {
    this.id = ''
    this.title = ''
    this.description = ''
    this.render = undefined
  }
}
