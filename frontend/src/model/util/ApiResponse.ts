export default interface ApiResponse {
  success: boolean
  msg: string
  data: unknown | unknown[] | undefined
}
