export default interface ApiResponse<T = unknown> {
  success: boolean
  msg: string
  data?: T
}
