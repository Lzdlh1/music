import api from './index'
import type { ApiResponse } from '@/types'

export interface AuthStatus {
  auth_enabled: boolean
  need_setup: boolean
}

export interface LoginResult {
  token: string
  user_id: string
  username: string
  role: string
}

export function getAuthStatus() {
  return api.get<ApiResponse<AuthStatus>>('/auth/status')
}

export function login(username: string, password: string) {
  return api.post<ApiResponse<LoginResult>>('/auth/login', { username, password })
}

export function setup(username: string, password: string) {
  return api.post<ApiResponse<LoginResult>>('/auth/setup', { username, password })
}

export function register(code: string, username: string, password: string) {
  return api.post<ApiResponse<LoginResult>>('/auth/register', { code, username, password })
}