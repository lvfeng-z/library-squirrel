/**
 * 将文件路径编码为资源请求 URL
 * 按路径分隔符拆分后逐段编码，避免 #、? 等特殊字符被浏览器误解析
 */
export function buildResourceUrl(filePath: string, queryString: string = ''): string {
  if (!filePath) return ''
  const segments = filePath.split(/[/\\]/)
  const encoded = segments.map(segment => encodeURIComponent(segment)).join('/')
  return `/resource/${encoded}${queryString}`
}
