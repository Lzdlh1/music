import api from './index'
import type { ApiResponse, StorageTarget, FileInfo } from '@/types'

export function listStorageTargets() {
  return api.get<ApiResponse<StorageTarget[]>>('/storage')
}

export function createStorageTarget(data: Partial<StorageTarget>) {
  return api.post<ApiResponse<StorageTarget>>('/storage', data)
}

export function updateStorageTarget(id: string, data: Partial<StorageTarget>) {
  return api.put(`/storage/${id}`, data)
}

export function deleteStorageTarget(id: string) {
  return api.delete(`/storage/${id}`)
}

export function testStorageTarget(id: string) {
  return api.post<ApiResponse<{ success: boolean }>>(`/storage/${id}/test`)
}

export function browseStorage(id: string, path = '/') {
  return api.get<ApiResponse<FileInfo[]>>(`/storage/${id}/browse`, {
    params: { path },
  })
}

/** 新建文件夹 */
export function mkdirStorage(id: string, path: string) {
  return api.post<ApiResponse<{ message: string }>>(`/storage/${id}/mkdir`, { path })
}

/** 重命名/移动 */
export function renameStorage(id: string, oldPath: string, newPath: string) {
  return api.post<ApiResponse<{ message: string }>>(`/storage/${id}/rename`, {
    old_path: oldPath,
    new_path: newPath,
  })
}

/** 删除文件/文件夹 */
export function deleteStorageFile(id: string, path: string) {
  return api.delete<ApiResponse<{ message: string }>>(`/storage/${id}/file`, {
    params: { path },
  })
}

/** 上传文件到指定目录（multipart） */
export function uploadStorageFile(id: string, dir: string, file: File, onProgress?: (percent: number) => void) {
  const form = new FormData()
  form.append('file', file)
  form.append('path', dir)
  return api.post<ApiResponse<{ message: string; data: { path: string } }>>(`/storage/${id}/upload`, form, {
    timeout: 0,
    onUploadProgress: (e) => {
      if (onProgress && e.total) onProgress(Math.round((e.loaded / e.total) * 100))
    },
  })
}

/** 网盘文件流播放 URL */
export function storageStreamUrl(id: string, path: string, download = false) {
  const params = new URLSearchParams({ path })
  if (download) params.set('download', '1')
  return `/api/v1/storage/${id}/stream?${params.toString()}`
}

/** 判断是否为可播放的音频文件 */
export function isAudioFile(name: string) {
  return /\.(mp3|flac|m4a|aac|ogg|opus|wav)$/i.test(name)
}

/** 判断是否为歌词文件 */
export function isLyricsFile(name: string) {
  return /\.lrc$/i.test(name)
}

// ---------- 移动云盘（139）登录 ----------

export interface Yun139LoginResult {
  authorization: string
  account: string
  auth_token: string
}

/** 发送短信验证码 */
export function yun139SendSms(account: string) {
  return api.post('/yun139/sms/send', { account })
}

/** 短信验证码登录 */
export function yun139SmsLogin(account: string, code: string, random: string, storageId?: string) {
  return api.post('/yun139/sms/login', { account, code, random, storage_id: storageId })
}

/** 账号密码登录 */
export function yun139PasswordLogin(account: string, password: string, storageId?: string) {
  return api.post('/yun139/password/login', { account, password, storage_id: storageId })
}

/** 开始扫码登录 */
export function yun139QrStart() {
  return api.post('/yun139/qr/start')
}

/** 轮询扫码登录状态 */
export function yun139QrPoll(sid: string, storageId?: string) {
  return api.post('/yun139/qr/poll', { sid, storage_id: storageId })
}
