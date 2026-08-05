import api from './index'
import type { ApiResponse, TrackResult, AvailableSource } from '@/types'

export function searchTracks(q: string, quality?: string, page = 1, size = 20) {
  return api.get<ApiResponse<TrackResult[]>>('/search', {
    params: { q, quality, page, size },
  })
}

export function getTrackSources(id: string) {
  return api.get<ApiResponse<AvailableSource[]>>(`/track/${id}/sources`)
}

export function getTrackLyrics(id: string) {
  return api.get(`/track/${id}/lyrics`)
}

export function getTrackCover(id: string) {
  return api.get(`/track/${id}/cover`)
}
