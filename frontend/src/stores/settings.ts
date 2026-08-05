import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getSettings, updateSettings } from '@/api/settings'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<Record<string, unknown>>({})
  const loading = ref(false)

  async function fetch() {
    loading.value = true
    try {
      const res = await getSettings()
      settings.value = res.data.data || {}
    } finally {
      loading.value = false
    }
  }

  async function save(data: Record<string, unknown>) {
    await updateSettings(data)
    Object.assign(settings.value, data)
  }

  return { settings, loading, fetch, save }
})
