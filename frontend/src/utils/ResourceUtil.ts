import type { ResourceFullDTO } from '@bindings/github.com/library-squirrel/backend/base/model/dto'
import { ResourceType, StoreRole } from '@renderer/constants/sectionCode.ts'

// 展示类型嗅探扩展名(仅 resource_type 未声明/unknown 时降级用)
const IMAGE_EXTENSIONS = ['.jpg', '.jpeg', '.png', '.webp', '.gif', '.bmp']
const VIDEO_EXTENSIONS = ['.mp4', '.webm', '.mkv', '.mov', '.avi']
const AUDIO_EXTENSIONS = ['.mp3', '.m4a', '.aac', '.flac', '.wav', '.ogg']
const DOCUMENT_EXTENSIONS = ['.pdf', '.docx', '.doc', '.txt', '.rtf']

function getFileExtension(filePath?: string): string {
  if (!filePath) return ''
  const idx = filePath.lastIndexOf('.')
  if (idx < 0) return ''
  return filePath.slice(idx).toLowerCase()
}

/**
 * 资源是否可合并：同时具备视频轨与音频轨。
 */
export function isResourceMergeable(resource: ResourceFullDTO | null | undefined): boolean {
  const stores = resource?.stores
  if (!stores || stores.length === 0) return false
  let hasVideo = false
  let hasAudio = false
  for (const rs of stores) {
    if (rs.storeType === StoreRole.VIDEO_TRACK) hasVideo = true
    else if (rs.storeType === StoreRole.AUDIO_TRACK) hasAudio = true
  }
  return hasVideo && hasAudio
}

/**
 * 资源展示类型：优先 resource.resourceType；未声明/unknown 时按 workStore 扩展名嗅探降级。
 */
export function getResourcePreviewType(resource: ResourceFullDTO | null | undefined): string {
  const declared = resource?.resourceType
  if (declared && declared !== ResourceType.UNKNOWN) {
    return declared
  }
  const ext = getFileExtension(resource?.workStore?.filePath)
  if (ext) {
    if (IMAGE_EXTENSIONS.includes(ext)) return ResourceType.IMAGE
    if (VIDEO_EXTENSIONS.includes(ext)) return ResourceType.VIDEO
    if (AUDIO_EXTENSIONS.includes(ext)) return ResourceType.AUDIO
    if (ext === '.md') return ResourceType.ARTICLE
    if (DOCUMENT_EXTENSIONS.includes(ext)) return ResourceType.DOCUMENT
  }
  return ResourceType.UNKNOWN
}

/**
 * 资源外部打开路径：纯消费后端派生的 workStore（ResolvePrimaryStore 已按 PrimaryRoles 选定展示主体）。
 */
export function getResourceOpenPath(resource: ResourceFullDTO | null | undefined): string {
  return resource?.workStore?.filePath ?? ''
}
