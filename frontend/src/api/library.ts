import api from './index'
import type { ApiResponse, LibraryItem } from '@/types'

export function listLibrary(q?: string, page = 1, size = 50) {
  return api.get<ApiResponse<LibraryItem[]>>('/library', {
    params: { q, page, size },
  })
}

export function getLibraryItem(id: string) {
  return api.get<ApiResponse<LibraryItem>>(`/library/${id}`)
}

export function deleteLibraryItem(id: string) {
  return api.delete(`/library/${id}`)
}

/** 音乐库歌曲流播放 URL（同源，可直接用于 <audio>） */
export function libraryStreamUrl(id: string) {
  return `/api/v1/library/${id}/stream`
}

/** 获取音乐库歌曲歌词（LRC） */
export function getLibraryLyrics(id: string) {
  return api.get<ApiResponse<{ lrc: string; source: string } | null>>(`/library/${id}/lyrics`)
}

/** 解析 LRC 歌词文本为带时间戳的行数组 */
export function parseLrc(lrc: string): { time: number; text: string }[] {
  if (!lrc) return []
  const lines: { time: number; text: string }[] = []
  const re = /\[(\d{1,2}):(\d{1,2})(?:[.:](\d{1,3}))?\]/g
  for (const raw of lrc.split('\n')) {
    const text = raw.replace(/\[.*?\]/g, '').trim()
    if (!text) continue
    let m: RegExpExecArray | null
    re.lastIndex = 0
    while ((m = re.exec(raw)) !== null) {
      const min = parseInt(m[1], 10)
      const sec = parseInt(m[2], 10)
      const frac = m[3] ? parseInt(m[3].padEnd(3, '0').slice(0, 3), 10) : 0
      lines.push({ time: min * 60000 + sec * 1000 + frac, text })
    }
  }
  return lines.sort((a, b) => a.time - b.time)
}
