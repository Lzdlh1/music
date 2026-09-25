import api from './index'
import type { ApiResponse, TrackResult, AvailableSource } from '@/types'

export function searchTracks(q: string, quality?: string, page = 1, size = 20) {
  return api.get<ApiResponse<TrackResult[]>>('/search', {
    params: { q, quality, page, size },
  })
}

export function getTrackSources(id: string) {
  // 曲目 ID 可能包含 : | 等字符（如咪咕源），必须编码后再拼入路径
  return api.get<ApiResponse<AvailableSource[]>>(`/track/${encodeURIComponent(id)}/sources`)
}

export function getTrackLyrics(id: string) {
  return api.get(`/track/${encodeURIComponent(id)}/lyrics`)
}

export function getTrackCover(id: string) {
  return api.get(`/track/${encodeURIComponent(id)}/cover`)
}
