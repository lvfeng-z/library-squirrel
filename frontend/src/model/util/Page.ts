import IPage from '@renderer/model/util/IPage.ts'

export default class Page<Data> implements IPage<Data> {
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
   * 数据
   */
  data: (Data | null)[]

  constructor(page?: IPage<Data>) {
    if (page === undefined) {
      this.pageNumber = 1
      this.pageSize = 10
      this.pageCount = 0
      this.dataCount = 0
      this.currentCount = 0
      this.data = []
    } else {
      this.pageNumber = page.pageNumber
      this.pageSize = page.pageSize
      this.pageCount = page.pageCount
      this.dataCount = page.dataCount
      this.currentCount = page.currentCount
      this.data = page.data
    }
  }

  /**
   * 返回一个指定类型的PageModel
   */
  public transform<T>(): Page<T> {
    const result = new Page<T>()
    result.pageNumber = this.pageNumber
    result.pageSize = this.pageSize
    result.pageCount = this.pageCount
    result.dataCount = this.dataCount
    result.currentCount = this.currentCount
    result.data = []

    return result
  }

  /**
   * 创建一个指定类型的副本
   */
  public copy<NewData>(): Page<NewData> {
    const result = new Page<NewData>()
    result.pageNumber = this.pageNumber
    result.pageSize = this.pageSize
    result.pageCount = this.pageCount
    result.dataCount = this.dataCount
    result.currentCount = this.currentCount
    result.data = []

    return result
  }
}
