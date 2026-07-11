import type { ResourceFullDTO } from '@bindings/github.com/lvfeng-z/library-squirrel-sdk/dto'

const STORE_TYPE_VIDEO_TRACK = 'videoTrack'
const STORE_TYPE_AUDIO_TRACK = 'audioTrack'
const STORE_TYPE_MERGED = 'merged'

/**
 * 资源是否可合并：同时具备视频轨与音频轨。
 */
export function isResourceMergeable(resource: ResourceFullDTO | null | undefined): boolean {
  const stores = resource?.stores
  if (!stores || stores.length === 0) return false
  let hasVideo = false
  let hasAudio = false
  for (const rs of stores) {
    if (rs.storeType === STORE_TYPE_VIDEO_TRACK) hasVideo = true
    else if (rs.storeType === STORE_TYPE_AUDIO_TRACK) hasAudio = true
  }
  return hasVideo && hasAudio
}

/**
 * 资源外部打开路径：优先合并产物，其次主资源。
 */
export function getResourceOpenPath(resource: ResourceFullDTO | null | undefined): string {
  const stores = resource?.stores
  if (stores) {
    for (const rs of stores) {
      if (rs.storeType === STORE_TYPE_MERGED && rs.store?.filePath) {
        return rs.store.filePath
      }
    }
  }
  return resource?.workStore?.filePath ?? ''
}
