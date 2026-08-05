import api from './index'

export interface PlaylistTrack {
  title: string
  artist: string
  album: string
  duration: number
}

export function parsePlaylistURL(url: string) {
  return api.post<{ data: PlaylistTrack[]; total: number }>('/playlist/parse-url', { url })
}

export function parsePlaylistText(text: string, format = 'text') {
  return api.post<{ data: PlaylistTrack[]; total: number }>('/playlist/parse-text', { text, format })
}

export function parsePlaylistFile(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return api.post<{ data: PlaylistTrack[]; total: number }>('/playlist/parse-file', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}
