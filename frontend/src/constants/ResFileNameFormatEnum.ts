export default class ResFileNameFormatEnum {
  token: string
  name: string
  description: string

  constructor(code: string, name: string, description: string) {
    this.token = code
    this.name = name
    this.description = description
  }

  static AUTHOR = new ResFileNameFormatEnum('${author}', '作者', '作者名称')
  static LOCAL_AUTHOR_NAME = new ResFileNameFormatEnum('${localAuthorName}', '本地作者名称', '本地作者名称')
  static SITE_AUTHOR_NAME = new ResFileNameFormatEnum('${siteAuthorName}', '站点作者名称', '站点作者名称')
  static SITE_AUTHOR_ID = new ResFileNameFormatEnum('${siteAuthorId}', '站点作者id', '站点作者id')
  static SITE_WORK_ID = new ResFileNameFormatEnum('${siteWorkId}', '站点作品id', '站点作品id')
  static SITE_WORK_NAME = new ResFileNameFormatEnum('${siteWorkName}', '站点作品名称', '站点作品名称')
  static DESCRIPTION = new ResFileNameFormatEnum('${description}', '作品描述', '作品描述')
  static UPLOAD_TIME_YEAR = new ResFileNameFormatEnum('${uploadTimeYear}', '上传时间-年', '上传时间-年')
  static UPLOAD_TIME_MONTH = new ResFileNameFormatEnum('${uploadTimeMonth}', '上传时间-月', '上传时间-月')
  static UPLOAD_TIME_DAY = new ResFileNameFormatEnum('${uploadTimeDay}', '上传时间-日', '上传时间-日')
  static UPLOAD_TIME_HOUR = new ResFileNameFormatEnum('${uploadTimeHour}', '上传时间-时', '上传时间-时')
  static UPLOAD_TIME_MINUTE = new ResFileNameFormatEnum('${uploadTimeMinute}', '上传时间-分', '上传时间-分')
  static UPLOAD_TIME_SECOND = new ResFileNameFormatEnum('${uploadTimeSecond}', '上传时间-秒', '上传时间-秒')
  static EXPORT_TIME_YEAR = new ResFileNameFormatEnum('${exportTimeYear}', '导出时间-年', '导出时间-年')
  static EXPORT_TIME_MONTH = new ResFileNameFormatEnum('${exportTimeMonth}', '导出时间-月', '导出时间-月')
  static EXPORT_TIME_DAY = new ResFileNameFormatEnum('${exportTimeDay}', '导出时间-日', '导出时间-日')
  static EXPORT_TIME_HOUR = new ResFileNameFormatEnum('${exportTimeHour}', '导出时间-时', '导出时间-时')
  static EXPORT_TIME_MINUTE = new ResFileNameFormatEnum('${exportTimeMinute}', '导出时间-分', '导出时间-分')
  static EXPORT_TIME_SECOND = new ResFileNameFormatEnum('${exportTimeSecond}', '导出时间-秒', '导出时间-秒')
}
