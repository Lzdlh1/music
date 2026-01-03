import { defineStore } from 'pinia'

export const useSettingsStore = defineStore('settings', {
  state: () => ({
    cmcookies: localStorage.getItem('cm_cookies') || ''
  }),
  actions: {
    setCookies(v: string) {
      this.cmcookies = v
      localStorage.setItem('cm_cookies', v)
    }
  }
})
