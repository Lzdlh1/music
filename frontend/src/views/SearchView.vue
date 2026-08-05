<script setup lang="ts">
import { ref } from 'vue'
import { NInput, NButton, NSpace, NSelect, NSpin, NEmpty } from 'naive-ui'
import { useSearchStore } from '@/stores/search'
import TrackCard from '@/components/search/TrackCard.vue'
import DownloadDialog from '@/components/search/DownloadDialog.vue'
import type { TrackResult } from '@/types'

const searchStore = useSearchStore()
const searchInput = ref('')
const qualityFilter = ref<string | null>(null)
const showDownload = ref(false)
const downloadTrack = ref<TrackResult | null>(null)

const qualityOptions = [
  { label: '全部', value: '' },
  { label: 'FLAC', value: 'flac' },
  { label: '320K', value: '320' },
  { label: '128K', value: '128' },
]

async function handleSearch() {
  if (!searchInput.value.trim()) return
  await searchStore.search(searchInput.value.trim(), qualityFilter.value || undefined)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') handleSearch()
}

function handleDownload(track: TrackResult) {
  downloadTrack.value = track
  showDownload.value = true
}
</script>

<template>
  <div class="search-page">
    <div class="search-header">
      <h1 class="page-title">搜索音乐</h1>
      <div class="search-bar">
        <n-input
          v-model:value="searchInput"
          placeholder="搜索歌曲、歌手、专辑..."
          size="large"
          clearable
          @keydown="handleKeydown"
        >
          <template #prefix>
            🔍
          </template>
        </n-input>
        <n-button type="primary" size="large" @click="handleSearch" :loading="searchStore.loading">
          搜索
        </n-button>
      </div>
      <n-space class="filters">
        <n-select
          v-model:value="qualityFilter"
          :options="qualityOptions"
          placeholder="音质"
          style="width: 120px"
          size="small"
        />
      </n-space>
    </div>

    <n-spin :show="searchStore.loading">
      <div v-if="searchStore.results.length" class="results-list">
        <TrackCard
          v-for="track in searchStore.results"
          :key="track.id"
          :track="track"
          @download="handleDownload"
        />
        <div class="load-more" v-if="searchStore.results.length < searchStore.total">
          <n-button @click="searchStore.loadMore()" :loading="searchStore.loading">
            加载更多
          </n-button>
        </div>
      </div>
      <n-empty
        v-else-if="!searchStore.loading && searchStore.keyword"
        description="未找到相关结果"
      />
    </n-spin>

    <DownloadDialog v-model:show="showDownload" :track="downloadTrack" />
  </div>
</template>

<style scoped>
.search-page {
  max-width: 900px;
  margin: 0 auto;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 16px;
}

.search-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
}

.filters {
  margin-bottom: 20px;
}

.results-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.load-more {
  text-align: center;
  padding: 16px;
}
</style>
