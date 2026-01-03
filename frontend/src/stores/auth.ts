import { defineStore } from 'pinia'
import api from '../api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || ''
  }),
  actions: {
    async login(username: string, password: string) {
      const res = await api.post('/login', { username, password })
      this.token = res.data.token
      localStorage.setItem('token', this.token)
    },
    logout() {
      this.token = ''
      localStorage.removeItem('token')
    }
  }
})
