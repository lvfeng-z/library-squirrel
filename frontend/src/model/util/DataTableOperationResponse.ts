// DataTable操作点击返回类型
interface DataTableOperationResponse<T> {
  id: string
  code: string
  data: T
}

export default DataTableOperationResponse
