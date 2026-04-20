export default interface IPage<Result, Data> {
  /**
   * 当前页码
   */
  pageNumber: number
  /**
   * 分页大小
   */
  pageSize: number
  /**
   * 总页数
   */
  pageCount: number
  /**
   * 数据总量
   */
  dataCount: number
  /**
   * 本页数据量
   */
  currentCount: number
  /**
   * 查询条件
   */
  query?: Data
  /**
   * 数据
   */
  data?: Result[]
}
