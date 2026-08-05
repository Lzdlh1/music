<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NInput, NDataTable, NSpace, NButton, NEmpty, NSelect } from 'naive-ui'
import type { LibraryItem } from '@/types'
import api from '@/api/index'

const items = ref<LibraryItem[]>([])
const total = ref(0)
const loading = ref(false)
const searchQuery = ref('')
const page = ref(1)

const columns = [
  { title: '#', key: 'index', width: 50, render: (_: unknown, index: number) => index + 1 },
  { title: '歌名', key: 'title', ellipsis: true },
  { title: '艺术家', key: 'artist', width: 150, ellipsis: true },
  { title: '专辑', key: 'album', width: 180, ellipsis: true },
  { title: '音质', key: 'quality', width: 80 },
  { title: '格式', key: 'format', width: 60 },
]

async function fetchLibrary() {
  loading.value = true
  try {
    const res = await api.get('/library', {
      params: { q: searchQuery.value, page: page.value, size: 50 },
    })
    items.value = res.data.data || []
    total.value = res.data.total || 0
  } finally {
    loading.value = false
  }
}

onMounted(fetchLibrary)
</script>

<template>
  <div class="library-page">
    <div class="library-header">
      <h1 class="page-title">音乐库 ({{ total }}首)</h1>
      <n-input
        v-model:value="searchQuery"
        placeholder="搜索库中歌曲..."
        style="width: 300px"
        clearable
        @update:value="fetchLibrary"
      />
    </div>

    <n-data-table
      v-if="items.length"
      :columns="columns"
      :data="items"
      :loading="loading"
      :row-key="(row: LibraryItem) => row.id"
      striped
    />
    <n-empty v-else-if="!loading" description="音乐库为空" />
  </div>
</template>

<style scoped>
.library-page {
  max-width: 1100px;
  margin: 0 auto;
}

.library-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
}
</style>
