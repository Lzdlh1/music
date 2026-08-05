// ========== 通用类型 ==========

export interface ApiResponse<T> {
  data: T
  error?: boolean
  message?: string
  total?: number
  page?: number
}

// ========== 音质 ==========

export type Quality = 0 | 128 | 320 | 999 | 9999

export const QualityLabels: Record<Quality, string> = {
  0: 'ANY',
  128: '128K',
  320: '320K',
  999: 'FLAC',
  9999: 'Hi-Res',
}

// ========== 搜索 ==========

export interface SearchQuery {
  keyword: string
  artist?: string
  album?: string
  quality?: Quality
  page?: number
  pageSize?: number
}

export interface TrackResult {
  id: string
  title: string
  artist: string
  album: string
  duration: number
  quality: Quality
  file_size: number
  source: string
  cover_url: string
  score: number
}

export interface TrackDetail {
  id: string
  title: string
  artist: string
  album_artist: string
  album: string
  track_no: number
  disc_no: number
  year: number
  genre: string
  duration: number
  cover_url: string
  source: string
}

export interface AvailableSource {
  source_name: string
  quality: Quality
  file_size: number
  format: string
  bit_rate: number
  sample_rate: number
  bit_depth: number
  score: number
  download_url: string
}

// ========== 任务 ==========

export type TaskStatus =
  | 'PENDING'
  | 'FETCHING_META'
  | 'DOWNLOADING'
  | 'PROCESSING'
  | 'UPLOADING'
  | 'DONE'
  | 'FAILED'
  | 'PAUSED'
  | 'CANCELLED'

export interface TaskProgress {
  stage: string
  percent: number
  speed: number
  downloaded: number
  total: number
  eta: number
  upload_progress?: Record<string, number>
}

export interface Task {
  id: string
  type: string
  status: TaskStatus
  priority: number
  track_info: TrackDetail
  selected_source?: AvailableSource
  upload_targets?: string[]
  progress?: TaskProgress
  error?: string
  retry_count: number
  created_at: string
  started_at?: string
  finished_at?: string
}

export interface TaskStats {
  PENDING: number
  DOWNLOADING: number
  PROCESSING: number
  UPLOADING: number
  DONE: number
  FAILED: number
  PAUSED: number
}

// ========== 存储 ==========

export type StorageType = 'webdav' | 'local' | 'sftp' | 's3' | 'onedrive' | 'aliyun' | 'gdrive'

export interface StorageTarget {
  id: string
  name: string
  type: StorageType
  config: Record<string, unknown>
  enabled: boolean
  created_at: string
}

export interface FileInfo {
  name: string
  path: string
  size: number
  is_dir: boolean
}

// ========== 音乐源 ==========

export interface MusicSourceConfig {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  priority: number
  enabled: boolean
}

// ========== 音乐库 ==========

export interface LibraryItem {
  id: string
  title: string
  artist: string
  album: string
  year: number
  genre: string
  quality: string
  format: string
  file_size: number
  duration: number
  source: string
  remote_paths?: Record<string, string>
  cover_url: string
  has_lyrics: boolean
  created_at: string
  updated_at: string
}

// ========== WebSocket ==========

export interface WSMessage {
  type: 'task_update' | 'task_done' | 'task_failed' | 'queue_stats' | 'tg_status'
  data: unknown
  timestamp: string
}

export interface TaskUpdateData {
  task_id: string
  status: TaskStatus
  progress: TaskProgress
}

// ========== 系统 ==========

export interface SystemInfo {
  version: string
  name: string
}
