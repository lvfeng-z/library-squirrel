import { PersistentStoreDTO, WorkSetWithCoverDTO } from '@bindings/github.com//lvfeng-z/library-squirrel-sdk/dto'
import WorkCardItem from '@renderer/model/dto/WorkCardItem.ts'

// 从 PersistentStore 提取有效宽高（width/height 均 > 0 才视为有效）
function dimensionFromStore(
  store: PersistentStoreDTO | null | undefined
): { width: number; height: number } | undefined {
  if (store && store.width > 0 && store.height > 0) {
    return { width: store.width, height: store.height }
  }
  return undefined
}

// 作品卡片维度：优先缩略图，其次原图
export function getWorkCardDimension(
  work: WorkCardItem
): { width: number; height: number } | undefined {
  const resource = work.resource
  if (!resource) {
    return undefined
  }
  return dimensionFromStore(resource.thumbnailStore) ?? dimensionFromStore(resource.workStore)
}

// 作品集卡片维度：优先封面缩略图，其次封面原图
export function getWorkSetCardDimension(
  ws: WorkSetWithCoverDTO
): { width: number; height: number } | undefined {
  const cover = ws.coverResource
  if (!cover) {
    return undefined
  }
  return dimensionFromStore(cover.thumbnailStore) ?? dimensionFromStore(cover.workStore)
}
