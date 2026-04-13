/**
 * 判断字符串是否为空白
 */
export function isBlank(input: string | null | undefined): input is undefined | null | '' {
  if (input === null || input === undefined) {
    return true
  }
  return /^\s*$/.test(input)
}

/**
 * 判断字符串是否不为空白
 */
export function isNotBlank(input: string | null | undefined): input is string {
  return !isBlank(input)
}

/**
 * 驼峰转蛇形
 */
export function camelToSnakeCase(str: string): string {
  return str.replace(/([a-z])([A-Z])/g, '$1_$2').toLowerCase()
}

/**
 * 蛇形转驼峰
 */
export function snakeToCamelCase(str: string): string {
  return str.replace(/(_\w)/g, (m) => m[1].toUpperCase())
}

/**
 * 如果字符串以指定前缀开头，则移除该前缀
 */
export function removePrefixIfPresent(str: string, target: string, caseSensitive: boolean = true): string {
  const trimmedStr = str.trim()

  let startsWithTarget: boolean

  if (caseSensitive) {
    startsWithTarget = trimmedStr.startsWith(target)
  } else {
    startsWithTarget = trimmedStr.toLowerCase().startsWith(target.toLowerCase())
  }

  if (startsWithTarget) {
    return trimmedStr.slice(target.length)
  } else {
    return str
  }
}

/**
 * 如果字符串不以指定前缀开头，则添加该前缀
 */
export function concatPrefixIfNotPresent(str: string, target: string, caseSensitive: boolean = true): string {
  const trimmedStr = str.trim()

  let startsWithTarget: boolean

  if (caseSensitive) {
    startsWithTarget = trimmedStr.startsWith(target)
  } else {
    startsWithTarget = trimmedStr.toLowerCase().startsWith(target.toLowerCase())
  }

  if (startsWithTarget) {
    return str
  } else {
    return target.concat(trimmedStr)
  }
}

/**
 * 从正则表达式匹配中创建对象数组
 */
export function createObjectsFromRegexMatches(regexPattern: string, targetString: string, propertiesArray: string[]): object[] {
  const regex = new RegExp(regexPattern, 'g')
  let match
  const resultObjects: object[] = []

  while ((match = regex.exec(targetString)) !== null) {
    if (match.length - 1 === propertiesArray.length) {
      const obj: Record<string, string> = {}
      propertiesArray.forEach((propertyName, index) => {
        obj[propertyName] = match[index + 1]
      })
      resultObjects.push(obj)
    } else {
      const msg = `捕获组数量与属性数组长度不匹配，请检查正则表达式与属性名数组是否匹配。regexPattern: ${regexPattern} ; propertiesArray: ${propertiesArray}`
      throw new Error(msg)
    }
  }

  return resultObjects
}
