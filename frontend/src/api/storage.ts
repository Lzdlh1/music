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
