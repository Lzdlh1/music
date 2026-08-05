import api from './index'

export function login(username: string, password: string) {
  return api.post<{ token: string; username: string }>('/auth/login', { username, password })
}

export function getAuthStatus() {
  return api.get<{ auth_enabled: boolean }>('/auth/status')
}
