import api from './index'
import type { ApiResponse } from '@/types'

export interface User {
  id: string
  username: string
  role: string
  disabled: boolean
  qq_limit: number
  qq_used: number
  download_dir: string
  default_storage: string
  created_at: string
}

export interface UserRow extends User {
  qq_used: number
}

export interface InviteKey {
  id: string
  code: string
  created_at: string
  used_by: string
  username?: string
  used_at?: string | null
}

export interface MyQuota {
  qq: { used: number; limit: number }
  items: { source_name: string; used: number; limit: number }[]
}

// ---- 当前用户 ----
export function getMe() {
  return api.get<ApiResponse<User>>('/users/me')
}

export function updateMe(body: { download_dir?: string; default_storage?: string; password?: string }) {
  return api.put<ApiResponse<{ message: string }>>('/users/me', body)
}

export function getMyQuota() {
  return api.get<ApiResponse<MyQuota>>('/users/me/quota')
}

// ---- 管理员 ----
export function listUsers() {
  return api.get<ApiResponse<UserRow[]>>('/users')
}

export function updateUser(
  id: string,
  body: { role?: string; disabled?: boolean; qq_limit?: number; default_storage?: string; download_dir?: string },
) {
  return api.put<ApiResponse<{ message: string }>>(`/users/${id}`, body)
}

export function createInvite() {
  return api.post<ApiResponse<InviteKey>>('/users/invite', {})
}

export function listInvites() {
  return api.get<ApiResponse<InviteKey[]>>('/users/invites')
}

export function deleteInvite(id: string) {
  return api.delete(`/users/invites/${id}`)
}