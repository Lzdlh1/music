import { defineStore } from 'pinia'

interface Session {
  token: string
  user_id: string
  username: string
  role: string
}

export const useAuthStore = defineStore('auth', {
  state: (): Session => ({
    token: localStorage.getItem('mf_token') || '',
    user_id: localStorage.getItem('mf_user_id') || '',
    username: localStorage.getItem('mf_username') || '',
    role: localStorage.getItem('mf_role') || '',
  }),
  getters: {
    loggedIn: (s) => !!s.token,
    isAdmin: (s) => s.role === 'admin',
  },
  actions: {
    setSession(t: Partial<Session> & { token: string }) {
      this.token = t.token
      if (t.user_id !== undefined) this.user_id = t.user_id
      if (t.username !== undefined) this.username = t.username
      if (t.role !== undefined) this.role = t.role
      localStorage.setItem('mf_token', this.token)
      if (this.user_id) localStorage.setItem('mf_user_id', this.user_id)
      if (this.username) localStorage.setItem('mf_username', this.username)
      if (this.role) localStorage.setItem('mf_role', this.role)
    },
    syncFromStorage() {
      this.token = localStorage.getItem('mf_token') || ''
      this.role = localStorage.getItem('mf_role') || 'user'
      this.username = localStorage.getItem('mf_username') || ''
      this.user_id = localStorage.getItem('mf_user_id') || ''
    },
    logout() {
      this.token = ''
      this.user_id = ''
      this.username = ''
      this.role = ''
      ;['mf_token', 'mf_user_id', 'mf_username', 'mf_role'].forEach((k) => localStorage.removeItem(k))
    },
  },
})