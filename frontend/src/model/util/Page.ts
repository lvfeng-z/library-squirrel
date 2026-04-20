import IPage from '@renderer/model/util/IPage.ts'

export default class Page<Query, Result> implements IPage<Query, Result> {
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
  query?: Query
  /**
   * 数据
   */
  data?: Result[]

  constructor(page?: IPage<Query, Result>) {
    if (page === undefined) {
      this.pageNumber = 1
      this.pageSize = 10
      this.pageCount = 0
      this.dataCount = 0
      this.currentCount = 0
      this.query = undefined
      this.data = undefined
    } else {
      this.pageNumber = page.pageNumber
      this.pageSize = page.pageSize
      this.pageCount = page.pageCount
      this.dataCount = page.dataCount
      this.currentCount = page.currentCount
      this.query = page.query
      this.data = page.data
    }
  }

  /**
   * 返回一个指定类型的PageModel
   */
  public transform<T>(): Page<Query, T> {
    const result = new Page<Query, T>()
    result.pageNumber = this.pageNumber
    result.pageSize = this.pageSize
    result.pageCount = this.pageCount
    result.dataCount = this.dataCount
    result.currentCount = this.currentCount
    result.query = this.query
    result.data = []

    return result
  }

  /**
   * 创建一个指定类型的副本
   */
  public copy<NewQuery, NewResult>(): Page<NewQuery, NewResult> {
    const result = new Page<NewQuery, NewResult>()
    result.pageNumber = this.pageNumber
    result.pageSize = this.pageSize
    result.pageCount = this.pageCount
    result.dataCount = this.dataCount
    result.currentCount = this.currentCount
    result.query = undefined
    result.data = undefined

    return result
  }
}
