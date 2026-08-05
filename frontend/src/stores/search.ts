import { defineStore } from 'pinia'
import { ref } from 'vue'
import { searchTracks } from '@/api/search'
import type { TrackResult } from '@/types'

export const useSearchStore = defineStore('search', () => {
  const results = ref<TrackResult[]>([])
  const loading = ref(false)
  const keyword = ref('')
  const total = ref(0)
  const page = ref(1)

  async function search(q: string, quality?: string) {
    keyword.value = q
    loading.value = true
    page.value = 1
    try {
      const res = await searchTracks(q, quality, 1)
      results.value = res.data.data || []
      total.value = res.data.total || 0
    } finally {
      loading.value = false
    }
  }

  async function loadMore() {
    if (loading.value) return
    loading.value = true
    page.value++
    try {
      const res = await searchTracks(keyword.value, undefined, page.value)
      const newResults = res.data.data || []
      results.value.push(...newResults)
    } finally {
      loading.value = false
    }
  }

  function clear() {
    results.value = []
    keyword.value = ''
    total.value = 0
    page.value = 1
  }

  return { results, loading, keyword, total, page, search, loadMore, clear }
})
