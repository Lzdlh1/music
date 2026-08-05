<script setup lang="ts">
import { NCard, NTag, NButton, NSpace } from 'naive-ui'
import type { TrackResult } from '@/types'

defineProps<{
  track: TrackResult
}>()

const emit = defineEmits<{
  download: [track: TrackResult]
}>()

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

function formatSize(bytes: number): string {
  if (bytes === 0) return ''
  const mb = bytes / (1024 * 1024)
  return `${mb.toFixed(1)}MB`
}

function qualityColor(quality: number): string {
  if (quality >= 999) return '#3b82f6'
  if (quality >= 320) return '#22c55e'
  if (quality >= 128) return '#eab308'
  return '#9ca3af'
}
</script>

<template>
  <n-card size="small" hoverable class="track-card">
    <div class="track-row">
      <div class="track-cover">
        <img
          v-if="track.cover_url"
          :src="track.cover_url"
          alt="cover"
          loading="lazy"
        />
        <div v-else class="cover-placeholder">🎵</div>
      </div>
      <div class="track-info">
        <div class="track-title">{{ track.title }}</div>
        <div class="track-artist">
          {{ track.artist }}
          <span v-if="track.album"> · {{ track.album }}</span>
        </div>
        <n-space size="small">
          <n-tag size="tiny" :bordered="false" round>
            {{ track.source }}
          </n-tag>
          <n-tag
            size="tiny"
            :bordered="false"
            round
            :color="{ textColor: qualityColor(track.quality) }"
          >
            {{ track.quality >= 999 ? 'FLAC' : track.quality + 'K' }}
          </n-tag>
          <span class="track-meta" v-if="track.duration">
            {{ formatDuration(track.duration) }}
          </span>
          <span class="track-meta" v-if="track.file_size">
            {{ formatSize(track.file_size) }}
          </span>
        </n-space>
      </div>
      <div class="track-actions">
        <n-button size="small" type="primary" @click="emit('download', track)">
          下载
        </n-button>
      </div>
    </div>
  </n-card>
</template>

<style scoped>
.track-card {
  border-radius: 12px;
}

.track-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.track-cover {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
  background: #f3f4f6;
}

.track-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.track-info {
  flex: 1;
  min-width: 0;
}

.track-title {
  font-weight: 600;
  font-size: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-artist {
  font-size: 12px;
  color: #999;
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.track-meta {
  font-size: 11px;
  color: #bbb;
}

.track-actions {
  flex-shrink: 0;
}
</style>
