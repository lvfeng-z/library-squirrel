/**
 * 插件加载时机类型
 */
export enum ActivationType {
  /** 主程序启动时加载 */
  STARTUP = 'startup',
  /** 按需加载 - 首次使用时加载 */
  LAZY = 'lazy',
  /** 手动加载 - 需要显式调用激活 */
  MANUAL = 'manual'
}
