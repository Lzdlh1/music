import api from './index'
import type { ApiResponse, Task, TaskStats } from '@/types'

export function createTask(data: {
  track_info: Record<string, unknown>
  selected_source: Record<string, unknown>
  upload_targets: string[]
  upload_dir?: string
}) {
  return api.post<ApiResponse<Task>>('/tasks', data)
}

export function listTasks(status?: string, page = 1, size = 20) {
  return api.get<ApiResponse<Task[]>>('/tasks', {
    params: { status, page, size },
  })
}

export function getTask(id: string) {
  return api.get<ApiResponse<Task>>(`/tasks/${id}`)
}

export function pauseTask(id: string) {
  return api.put(`/tasks/${id}/pause`)
}

export function resumeTask(id: string) {
  return api.put(`/tasks/${id}/resume`)
}

export function cancelTask(id: string) {
  return api.delete(`/tasks/${id}`)
}

export function getTaskStats() {
  return api.get<ApiResponse<TaskStats>>('/tasks/stats')
}

export function batchCreateTasks(tasks: Array<{
  track_info: Record<string, unknown>
  selected_source: Record<string, unknown>
  upload_targets: string[]
}>) {
  return api.post<ApiResponse<Task[]>>('/tasks/batch', { tasks })
}
