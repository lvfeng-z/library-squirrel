// 板块代码（与后端 taskManager.parseSections 契约一致：A=1/B=2/C=3）
export const SectionCode = {
  // 板块 A：作品信息
  WORK_INFO: 1,
  // 板块 B：资源文件
  RESOURCE: 2,
  // 板块 C：封面
  THUMBNAIL: 3
} as const

// 全部板块代码（用户不勾选任何板块时下发，等价完整重执行）
export const ALL_SECTIONS: number[] = [SectionCode.WORK_INFO, SectionCode.RESOURCE, SectionCode.THUMBNAIL]
