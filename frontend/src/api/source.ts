import api from './index'
import type { ApiResponse, MusicSourceConfig } from '@/types'

export function listSources() {
  return api.get<ApiResponse<MusicSourceConfig[]>>('/sources')
}

export function createSource(data: Partial<MusicSourceConfig>) {
  return api.post<ApiResponse<MusicSourceConfig>>('/sources', data)
}

export function updateSource(id: string, data: Partial<MusicSourceConfig>) {
  return api.put(`/sources/${id}`, data)
}

export function deleteSource(id: string) {
  return api.delete(`/sources/${id}`)
}

export function testSource(id: string) {
  return api.post<ApiResponse<{ success: boolean }>>(`/sources/${id}/test`)
}

/** QQ 音乐源本月下载额度使用情况（本地统计） */
export function getQQQuota(name: string) {
  return api.get<ApiResponse<{ name: string; month: string; used: number; limit: number }>>('/sources/qq/quota', {
    params: { name },
  })
}
