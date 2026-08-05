<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  NModal, NButton, NSpace, NTag, NCard, NSpin,
  NCheckbox, NCheckboxGroup, NEmpty, useMessage
} from 'naive-ui'
import { getTrackSources } from '@/api/search'
import { listStorageTargets } from '@/api/storage'
import { createTask } from '@/api/task'
import type { TrackResult, AvailableSource, StorageTarget } from '@/types'

const props = defineProps<{
  show: boolean
  track: TrackResult | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const message = useMessage()
const loading = ref(false)
const submitting = ref(false)
const sources = ref<AvailableSource[]>([])
const storageTargets = ref<StorageTarget[]>([])
const selectedSourceIdx = ref<number>(0)
const selectedTargets = ref<string[]>([])

const visible = computed({
  get: () => props.show,
  set: (v) => emit('update:show', v),
})

onMounted(async () => {
  const res = await listStorageTargets()
  storageTargets.value = (res.data.data || []).filter((t: StorageTarget) => t.enabled)
  // 默认全选
  selectedTargets.value = storageTargets.value.map((t) => t.id)
})

async function loadSources() {
  if (!props.track) return
  loading.value = true
  try {
    const res = await getTrackSources(props.track.id)
    sources.value = res.data.data || []
    selectedSourceIdx.value = 0
  } catch {
    sources.value = []
  } finally {
    loading.value = false
  }
}

function onOpen() {
  loadSources()
}

function qualityLabel(q: number): string {
  if (q >= 9999) return 'Hi-Res'
  if (q >= 999) return 'FLAC'
  if (q >= 320) return '320K'
  if (q >= 128) return '128K'
  return `${q}K`
}

function formatSize(bytes: number): string {
  if (!bytes) return ''
  const mb = bytes / (1024 * 1024)
  return `${mb.toFixed(1)} MB`
}

async function handleDownload() {
  if (!props.track) return

  submitting.value = true
  try {
    const trackInfo: Record<string, unknown> = {
      id: props.track.id,
      title: props.track.title,
      artist: props.track.artist,
      album: props.track.album,
      duration: props.track.duration,
      cover_url: props.track.cover_url,
      source: props.track.source,
    }

    const selected = sources.value[selectedSourceIdx.value]
    const selectedSource: Record<string, unknown> = selected
      ? {
          source_name: selected.source_name,
          quality: selected.quality,
          format: selected.format,
        }
      : {
          source_name: props.track.source,
          quality: props.track.quality,
          format: props.track.quality >= 999 ? 'flac' : 'mp3',
        }

    await createTask({
      track_info: trackInfo,
      selected_source: selectedSource,
      upload_targets: selectedTargets.value,
    })
    message.success(`已添加下载: ${props.track.title}`)
    visible.value = false
  } catch (e: any) {
    message.error(e.response?.data?.message || '创建任务失败')
  } finally {
    submitting.value = false
  }
}

async function handleQuickDownload() {
  if (!props.track) return
  submitting.value = true
  try {
    await createTask({
      track_info: {
        id: props.track.id,
        title: props.track.title,
        artist: props.track.artist,
        album: props.track.album,
        duration: props.track.duration,
        cover_url: props.track.cover_url,
        source: props.track.source,
      },
      selected_source: {
        source_name: props.track.source,
        quality: props.track.quality,
        format: props.track.quality >= 999 ? 'flac' : 'mp3',
      },
      upload_targets: selectedTargets.value,
    })
    message.success(`已添加下载: ${props.track.title}`)
    visible.value = false
  } catch (e: any) {
    message.error(e.response?.data?.message || '创建任务失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :title="`下载: ${track?.title || ''}`"
    style="width: 520px; max-width: 95vw;"
    @after-enter="onOpen"
  >
    <div v-if="track" class="download-dialog">
      <!-- 曲目信息 -->
      <div class="track-summary">
        <strong>{{ track.title }}</strong>
        <span class="meta">{{ track.artist }}<template v-if="track.album"> · {{ track.album }}</template></span>
      </div>

      <!-- 可用源 -->
      <div class="section">
        <div class="section-title">可用来源</div>
        <n-spin :show="loading" size="small">
          <div v-if="sources.length" class="source-list">
            <div
              v-for="(src, idx) in sources"
              :key="idx"
              class="source-item"
              :class="{ selected: selectedSourceIdx === idx }"
              @click="selectedSourceIdx = idx"
            >
              <div>
                <n-tag size="tiny" round>{{ src.source_name }}</n-tag>
                <n-tag size="tiny" round type="info" style="margin-left: 4px;">
                  {{ qualityLabel(src.quality) }}
                </n-tag>
                <span v-if="src.format" class="fmt">{{ src.format.toUpperCase() }}</span>
              </div>
              <span class="size">{{ formatSize(src.file_size) }}</span>
            </div>
          </div>
          <div v-else-if="!loading" class="no-sources">
            <p>未获取到其他来源，将使用搜索结果默认来源</p>
          </div>
        </n-spin>
      </div>

      <!-- 存储目标 -->
      <div class="section" v-if="storageTargets.length">
        <div class="section-title">上传到</div>
        <n-checkbox-group v-model:value="selectedTargets">
          <n-space>
            <n-checkbox v-for="t in storageTargets" :key="t.id" :value="t.id">
              {{ t.name }} ({{ t.type }})
            </n-checkbox>
          </n-space>
        </n-checkbox-group>
      </div>

      <!-- 操作 -->
      <div class="actions">
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" @click="handleQuickDownload" :loading="submitting" v-if="sources.length === 0 && !loading">
          直接下载
        </n-button>
        <n-button type="primary" @click="handleDownload" :loading="submitting" v-else :disabled="loading">
          下载选中
        </n-button>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.download-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.track-summary {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.track-summary .meta {
  color: #999;
  font-size: 13px;
}

.section-title {
  font-weight: 600;
  margin-bottom: 8px;
  font-size: 14px;
}

.source-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.source-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  cursor: pointer;
  transition: all 0.15s;
}
.source-item:hover {
  border-color: #6366f1;
}
.source-item.selected {
  border-color: #6366f1;
  background: #eef2ff;
}

.fmt {
  margin-left: 6px;
  font-size: 12px;
  color: #999;
}
.size {
  font-size: 12px;
  color: #666;
}

.no-sources {
  text-align: center;
  color: #999;
  padding: 12px;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
</style>
