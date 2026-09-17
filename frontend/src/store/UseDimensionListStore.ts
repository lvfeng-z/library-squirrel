import { defineStore } from 'pinia'
import { AuthorRole, TagNamespace } from '@bindings/github.com/library-squirrel/backend/base/model/entity/models'
import { BUILTIN_NAMESPACES } from '@renderer/constants/namespace.ts'
import { authorRoleList } from '@renderer/apis/http/wrappers/authorRole'
import { tagNamespaceList } from '@renderer/apis/http/wrappers/tagNamespace'
import { isNullish } from '@renderer/utils/CommonUtil.ts'

/**
 * 关联级维度清单（tag namespace / author role）全局缓存：
 * 后端清单表（内置常量启动投影 + 用户自设/插件声明 find-or-create）的唯一前端消费面，
 * 供各维度选择器分组展示候选（内置/用户/插件分组、last_use 降序让常用靠前）。
 * 清单是「见过的值」而非约束——选择器保留 allow-create 自定义开口，清单外值可直接输入。
 * 两维度清单结构同构、各自独立拉取（领域语义不同，不共用一次拉取）。
 */

/** 维度清单来源三态（与后端 backend/base/constant/dimension.go 对齐：数值即优先序 builtin>user>plugin） */
export const DimensionOrigin = {
  PLUGIN: 0,
  USER: 1,
  BUILTIN: 2
} as const

/** 分组展示形态（el-option-group 渲染）：label=分组名（内置/用户/插件） */
interface DimensionOptionGroup<T> {
  label: string
  options: T[]
}

/** 分组定义：按 origin 优先序排列（内置 > 用户 > 插件），空组不展示 */
const ORIGIN_GROUPS: { label: string; origin: number }[] = [
  { label: '内置', origin: DimensionOrigin.BUILTIN },
  { label: '用户', origin: DimensionOrigin.USER },
  { label: '插件', origin: DimensionOrigin.PLUGIN }
]

/** 清单行分组：组间按 origin 优先序、组内 last_use 降序（常用靠前）再按 value 升序稳定排序 */
function groupDimensionRows<T extends { value: string; origin: number; lastUse: number }>(rows: T[]): DimensionOptionGroup<T>[] {
  return ORIGIN_GROUPS.map(({ label, origin }) => ({
    label,
    options: rows
      .filter((row) => row.origin === origin)
      .sort((a, b) => b.lastUse - a.lastUse || (a.value < b.value ? -1 : a.value > b.value ? 1 : 0))
  })).filter((group) => group.options.length > 0)
}

/** 值 → 显示名：清单行 label 非空取 label，否则显示 value 本身；'/' 连接的多值串逐段解析（同作品同实体多维度值的聚合展示形态） */
function resolveDimensionLabel<T extends { value: string; label: string }>(rows: T[], value: string): string {
  if (!value) {
    return ''
  }
  return value
    .split('/')
    .map((part) => rows.find((row) => row.value === part)?.label || part)
    .join('/')
}

export const useDimensionListStore = defineStore('dimensionList', {
  state: (): { namespaces: TagNamespace[]; roles: AuthorRole[]; nsLoadAttempted: boolean; roleLoadAttempted: boolean } => {
    return {
      namespaces: [],
      roles: [],
      nsLoadAttempted: false,
      roleLoadAttempted: false
    }
  },
  getters: {
    /** namespace 候选分组（内置/用户/插件，组内 last_use 降序） */
    nsGroups(state): DimensionOptionGroup<TagNamespace>[] {
      return groupDimensionRows(state.namespaces)
    },
    /** role 候选分组（内置集为空，实际分组=用户用过的 + 插件声明的） */
    roleGroups(state): DimensionOptionGroup<AuthorRole>[] {
      return groupDimensionRows(state.roles)
    },
    /** namespace 值 → 显示名（支持 '/' 连接多值串）；清单未含的值显示原值 */
    nsLabelOf(state): (value: string) => string {
      return (value: string) => resolveDimensionLabel(state.namespaces, value)
    },
    /** role 值 → 显示名（支持 '/' 连接多值串）；清单未含的值显示原值 */
    roleLabelOf(state): (value: string) => string {
      return (value: string) => resolveDimensionLabel(state.roles, value)
    }
  },
  actions: {
    /** 拉取 namespace 清单（会话内一次，幂等）；失败降级为前端内置常量兜底（自定义开口不受影响） */
    async loadNamespaces(): Promise<void> {
      if (this.nsLoadAttempted) {
        return
      }
      this.nsLoadAttempted = true
      try {
        const result = await tagNamespaceList()
        this.namespaces = result.data.filter((row): row is TagNamespace => !isNullish(row))
      } catch (e) {
        console.error('拉取 namespace 清单失败，降级为内置常量兜底', e)
        this.namespaces = BUILTIN_NAMESPACES.map(
          (item) => new TagNamespace({ value: item.value, label: item.label, origin: DimensionOrigin.BUILTIN, lastUse: 0 })
        )
      }
    },
    /** 拉取 role 清单（会话内一次，幂等）；失败降级为空候选（自定义开口不受影响） */
    async loadRoles(): Promise<void> {
      if (this.roleLoadAttempted) {
        return
      }
      this.roleLoadAttempted = true
      try {
        const result = await authorRoleList()
        this.roles = result.data.filter((row): row is AuthorRole => !isNullish(row))
      } catch (e) {
        console.error('拉取 role 清单失败，降级为空候选', e)
        this.roles = []
      }
    }
  }
})
