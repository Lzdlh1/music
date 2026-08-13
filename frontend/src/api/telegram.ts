import api from './index'
import type { ApiResponse } from '@/types'

export interface TGBot {
  id: string
  name: string
  username: string
  config: Record<string, unknown>
  priority: number
  enabled: boolean
  success_rate: number
  last_tested?: string
}

export interface TGAccount {
  id: string
  phone: string
  username: string
  session_path: string
  status: string
  created_at: string
}

export interface TGChannel {
  id: string
  chat_id: number
  title: string
  username: string
  enabled: boolean
  file_count: number
  created_at: string
}

export interface TGChannelFile {
  id: string
  channel_id: string
  chat_id: number
  message_id: number
  file_id: string
  file_unique_id: string
  file_name: string
  file_size: number
  mime_type: string
  duration: number
  title: string
  artist: string
  caption: string
  downloaded: boolean
  posted_at: string
  created_at: string
}

// Bot API
export function listBots() {
  return api.get<ApiResponse<TGBot[]>>('/telegram/bots')
}

export function createBot(data: { name: string; username: string; config: Record<string, unknown>; priority?: number }) {
  return api.post<ApiResponse<TGBot>>('/telegram/bots', data)
}

export function updateBot(id: string, data: Partial<TGBot>) {
  return api.put(`/telegram/bots/${id}`, data)
}

export function deleteBot(id: string) {
  return api.delete(`/telegram/bots/${id}`)
}

export function testBot(id: string, token: string) {
  return api.post<ApiResponse<{ success: boolean; data?: { id: number; username: string } }>>(`/telegram/bots/${id}/test`, { token })
}

export function testBotToken(token: string) {
  return api.post<ApiResponse<{ success: boolean; data?: { id: number; username: string } }>>('/telegram/bots/test', { token })
}

// Account API
export function listAccounts() {
  return api.get<ApiResponse<TGAccount[]>>('/telegram/accounts')
}

export function createAccount(data: { phone: string; username?: string }) {
  return api.post<ApiResponse<TGAccount>>('/telegram/accounts', data)
}

export function deleteAccount(id: string) {
  return api.delete(`/telegram/accounts/${id}`)
}

export function startAccount(id: string) {
  return api.post(`/telegram/accounts/${id}/start`)
}

export function submitCode(id: string, code: string) {
  return api.post(`/telegram/accounts/${id}/code`, { code })
}

export function submitPassword(id: string, password: string) {
  return api.post(`/telegram/accounts/${id}/password`, { password })
}

// Channel API
export function listChannels() {
  return api.get<ApiResponse<TGChannel[]>>('/telegram/channels')
}

export function addChannel(chatId: string) {
  return api.post<ApiResponse<TGChannel>>('/telegram/channels', { chat_id: chatId })
}

export function removeChannel(id: string) {
  return api.delete(`/telegram/channels/${id}`)
}

export function toggleChannel(id: string, enabled: boolean) {
  return api.put(`/telegram/channels/${id}/toggle`, { enabled })
}

export function listChannelFiles(channelId: string, params?: { page?: number; page_size?: number; keyword?: string }) {
  return api.get(`/telegram/channels/${channelId}/files`, { params })
}

export function listAllChannelFiles(params?: { page?: number; page_size?: number; keyword?: string; channel_id?: string }) {
  return api.get('/telegram/channels/files', { params })
}

export function saveFileToLibrary(fileId: string) {
  return api.post(`/telegram/channels/files/${fileId}/save`)
}

export function scanChannelHistory(id: string) {
  return api.post(`/telegram/channels/${id}/scan`)
}
