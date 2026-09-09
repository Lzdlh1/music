import api from './index'
import type { ApiResponse, LibraryItem } from '@/types'

/** 在线试听流 URL（query token 鉴权） */
export function trackStreamUrl(trackId: string) {
  const base = `/api/v1/track/${encodeURIComponent(trackId)}/stream`
  const token = localStorage.getItem('mf_token')
  return token ? `${base}?token=${encodeURIComponent(token)}` : base
}

export function favoriteTrack(track: { track_id: string; title?: string; artist?: string; album?: string; duration?: number; cover_url?: string }) {
  return api.post<ApiResponse<LibraryItem>>('/library/favorite', track)
}

export function unfavoriteTrack(id: string) {
  return api.delete<ApiResponse<{ message: string }>>(`/library/favorite/${id}`)
}