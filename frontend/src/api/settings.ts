import api from './index'

export function getSettings() {
  return api.get('/settings')
}

export function updateSettings(data: Record<string, unknown>) {
  return api.put('/settings', data)
}

export function getSetting(key: string) {
  return api.get(`/settings/${key}`)
}

export function updateSetting(key: string, value: unknown) {
  return api.put(`/settings/${key}`, value)
}

export function getSystemInfo() {
  return api.get('/system/info')
}
