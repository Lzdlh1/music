<script setup lang="ts">
import { ref } from 'vue'
import { NInput, NButton, NSpace, NSelect, NSpin, NEmpty, NCard, NTag, useMessage } from 'naive-ui'
import { useSearchStore } from '@/stores/search'
import TrackCard from '@/components/search/TrackCard.vue'
import DownloadDialog from '@/components/search/DownloadDialog.vue'
import { listAllChannelFiles, saveFileToLibrary, type TGChannelFile } from '@/api/telegram'
import type { TrackResult } from '@/types'

const searchStore = useSearchStore()
const message = useMessage()
const searchInput = ref('')
const qualityFilter = ref<string | null>(null)
const showDownload = ref(false)
const downloadTrack = ref<TrackResult | null>(null)

// TG 频道资源
const tgFiles = ref<TGChannelFile[]>([])
const tgTotal = ref(0)
const tgLoading = ref(false)

const qualityOptions = [
  { label: '全部', value: '' },
  { label: 'FLAC', value: 'flac' },
  { label: '320K', value: '320' },
  { label: '128K', value: '128' },
]

async function searchTG(keyword: string) {
  tgLoading.value = true
  try {
    const res = await listAllChannelFiles({ page: 1, page_size: 20, keyword })
    const result = res.data as any
    tgFiles.value = result.data || []
    tgTotal.value = result.total || 0
  } catch {
    tgFiles.value = []
    tgTotal.value = 0
  } finally {
    tgLoading.value = false
  }
}

async function handleSearch() {
  if (!searchInput.value.trim()) return
  const q = searchInput.value.trim()
  searchStore.search(q, qualityFilter.value || undefined)
  searchTG(q)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') handleSearch()
}

function handleDownload(track: TrackResult) {
  downloadTrack.value = track
  showDownload.value = true
}

function handleTGDownload(file: TGChannelFile) {
  window.open(`/api/v1/telegram/channels/files/${file.id}/download`, '_blank')
}

async function handleTGSave(file: TGChannelFile) {
  try {
    const res = await saveFileToLibrary(file.id)
    const result = (res.data as any)
    if (result.success) {
      message.success('已加入下载队列')
      file.downloaded = true
    } else {
      message.error(result.message || '保存失败')
    }
  } catch {
    message.error('操作失败')
  }
}

function formatSize(bytes: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

function formatDuration(seconds: number): string {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

function tgQuality(file: TGChannelFile): number {
  const name = (file.file_name || file.title || '').toLowerCase()
  if (name.includes('.flac')) return 999
  if (name.includes('.wav') || name.includes('.ape') || name.includes('.dff') || name.includes('.dsf')) return 999
  if (name.includes('.m4a') || name.includes('.aac')) return 320
  return 320
}

function tgFormatLabel(file: TGChannelFile): string {
  const q = tgQuality(file)
  return q >= 999 ? 'FLAC' : q + 'K'
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

    <!-- TG 频道资源 -->
    <n-card
      v-if="searchStore.keyword && (tgFiles.length > 0 || tgLoading)"
      size="small"
      class="tg-section"
    >
      <template #header>
        <div style="display: flex; align-items: center; gap: 8px;">
          <span>Telegram 频道资源</span>
          <n-tag size="tiny" round type="info">{{ tgTotal }} 个</n-tag>
          <span v-if="tgLoading" style="font-size: 12px; color: #999;">搜索中...</span>
        </div>
      </template>

      <div v-if="tgFiles.length === 0 && !tgLoading" style="text-align: center; color: #999; padding: 16px 0;">
        <n-empty description="TG 频道中暂无匹配文件" />
      </div>

      <div v-for="file in tgFiles" :key="file.id" class="tg-file-item">
        <div class="tg-file-info">
          <div class="tg-file-title">
            {{ file.title || file.file_name || '未知标题' }}
            <n-tag v-if="file.downloaded" size="tiny" type="success" round style="margin-left: 6px;">已下载</n-tag>
          </div>
          <div class="tg-file-meta">
            <span v-if="file.artist">{{ file.artist }}</span>
            <span v-if="file.duration">{{ formatDuration(file.duration) }}</span>
            <span>{{ formatSize(file.file_size) }}</span>
            <n-tag size="tiny" :bordered="false" round>{{ tgFormatLabel(file) }}</n-tag>
          </div>
        </div>
        <n-space>
          <n-button size="tiny" @click="handleTGDownload(file)">下载</n-button>
          <n-button size="tiny" type="primary" :disabled="file.downloaded" @click="handleTGSave(file)">
            保存到库
          </n-button>
        </n-space>
      </div>
    </n-card>

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

.tg-section {
  margin-top: 20px;
  border-radius: 12px;
}

.tg-file-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 4px;
  border-bottom: 1px solid #f0f0f0;
}
.tg-file-item:last-child {
  border-bottom: none;
}
.tg-file-info {
  flex: 1;
  min-width: 0;
}
.tg-file-title {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tg-file-meta {
  font-size: 12px;
  color: #999;
  margin-top: 2px;
  display: flex;
  align-items: center;
  gap: 12px;
}
</style>
