import type { TourDefinition } from '@renderer/model/tour/TourDefinition'

/**
 * 内置向导定义
 *
 * 新增向导只需在此数组中添加 TourDefinition，无需改动 store 或页面（除非该向导
 * 需要高亮新元素，此时目标页面需通过 useTourTargets 注册对应 targetKey）。
 */
export const builtinTours: TourDefinition[] = [
  {
    id: 'workdir-setup',
    name: '工作目录配置引导',
    description: '工作目录未配置的影响与配置入口',
    steps: [
      {
        target: { route: 'mainPage' },
        title: '重要提示：工作目录',
        description: '若不配置工作目录，本软件无法正常使用——资源下载、备份与文件服务均不可用',
      },
      {
        target: { route: 'settings', targetKey: 'settings.workdirInput' },
        title: '工作目录',
        description: '在这里设置资源库的根目录，本软件管理的所有资源都会被保存到这个目录下，请确保这个目录有足够的空间，并且非必要的情况下不要更改此项',
      },
    ],
  },
  {
    id: 'task-creation',
    name: '任务创建引导',
    description: '认识本地导入与站点下载两个任务创建入口',
    steps: [
      {
        target: { route: 'taskManage', targetKey: 'taskManage.localImportButton' },
        title: '从本地导入',
        description: '点击这里可以选择本地目录或单个文件来创建任务',
      },
      {
        target: { route: 'taskManage', targetKey: 'taskManage.siteDownloadButton' },
        title: '从站点下载',
        description: '粘贴受支持的站点 URL 创建下载任务，可通过安装插件扩展受支持的 URL',
      },
    ],
  },
  {
    id: 'local-tag-intro',
    name: '本地标签介绍',
    description: '认识本地标签页面',
    steps: [
      {
        target: { route: 'localTagManage', targetKey: 'localTagManage.localTagTable' },
        title: '本地标签列表',
        description: '这里是本地标签列表，库中所有本地标签都在此统一管理，可新增、编辑、删除标签',
        placement: 'right',
      },
      {
        target: { route: 'localTagManage', targetKey: 'localTagManage.siteTagExchange' },
        title: '站点标签绑定',
        description: '选中左侧本地标签后，在此管理绑定到它的站点标签，可在「已绑定」与「未绑定」之间转移，建立本地标签与站点标签的关联',
        placement: 'left',
      },
    ],
  },
  // 「定位指定标签」向导示例：演示跳转并定位到某条数据，
  // 需在 LocalTagManage 页接入 useTourTargets('localTagManage.table') 与 useTourReady 后启用。
  // {
  //   id: 'locate-tag',
  //   name: '定位指定标签',
  //   description: '演示跳转到本地标签页并定位到某条数据',
  //   steps: [
  //     {
  //       target: { route: 'localTagManage', targetKey: 'localTagManage.table' },
  //       description: '正在为您定位标签…',
  //       data: { kind: 'tag', tagId: 123, scope: 'local' },
  //     },
  //   ],
  // },
]

/**
 * 批量注册内置向导
 */
export function registerBuiltinTours(register: (def: TourDefinition) => void) {
  builtinTours.forEach((def) => register(def))
}
