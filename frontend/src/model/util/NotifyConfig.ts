export default interface NotifyConfig {
  msg: string
  type: 'primary' | 'success' | 'warning' | 'info' | 'error'
  maxRow?: number
  duration?: number
}
