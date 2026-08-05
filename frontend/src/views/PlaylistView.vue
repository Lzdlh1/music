<script setup lang="ts">
import { ref } from 'vue'
import { NInput, NButton, NCard, NEmpty, NDataTable, NSpace, NUpload, useMessage, type UploadFileInfo } from 'naive-ui'
import { parsePlaylistURL, parsePlaylistText, parsePlaylistFile, type PlaylistTrack } from '@/api/playlist'
import { createTask } from '@/api/task'

const message = useMessage()
const playlistUrl = ref('')
const loading = ref(false)
const tracks = ref<PlaylistTrack[]>([])

const columns = [
  { title: '#', key: 'index', width: 50, render: (_: PlaylistTrack, index: number) => index + 1 },
  { title: '歌名', key: 'title' },
  { title: '歌手', key: 'artist' },
  { title: '专辑', key: 'album' },
]

async function handleParseURL() {
  if (!playlistUrl.value.trim()) return
  loading.value = true
  try {
    const res = await parsePlaylistURL(playlistUrl.value)
    tracks.value = res.data.data || []
    message.success(`解析到 ${tracks.value.length} 首曲目`)
  } catch (e: any) {
    message.error(e.response?.data?.message || '解析失败')
  } finally {
    loading.value = false
  }
}

async function handleFileUpload(options: { file: UploadFileInfo }) {
  const file = options.file.file
  if (!file) return
  loading.value = true
  try {
    const res = await parsePlaylistFile(file)
    tracks.value = res.data.data || []
    message.success(`解析到 ${tracks.value.length} 首曲目`)
  } catch (e: any) {
    message.error(e.response?.data?.message || '文件解析失败')
  } finally {
    loading.value = false
  }
}

async function handleDownloadAll() {
  if (tracks.value.length === 0) return
  loading.value = true
  try {
    for (const track of tracks.value) {
      await createTask({
        track_info: {
          title: track.title,
          artist: track.artist,
          album: track.album,
          duration: track.duration,
          id: `${track.artist}:${track.title}`,
        },
        selected_source: {},
        upload_targets: [],
      })
    }
    message.success(`已添加 ${tracks.value.length} 个下载任务`)
  } catch (e: any) {
    message.error('批量添加任务失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="playlist-page">
    <h1 class="page-title">歌单导入</h1>
    <n-card>
      <p>输入歌单链接或上传文件：</p>
      <div class="import-row">
        <n-input
          v-model:value="playlistUrl"
          placeholder="粘贴网易云/QQ/Spotify 歌单链接..."
          size="large"
          @keydown.enter="handleParseURL"
        />
        <n-button type="primary" size="large" @click="handleParseURL" :loading="loading">
          解析
        </n-button>
      </div>
      <div class="upload-row">
        <n-upload
          :show-file-list="false"
          accept=".m3u,.m3u8,.csv,.txt"
          :custom-request="handleFileUpload as any"
        >
          <n-button>选择 M3U / CSV / TXT 文件</n-button>
        </n-upload>
      </div>
    </n-card>

    <template v-if="tracks.length > 0">
      <div style="display: flex; justify-content: space-between; align-items: center; margin: 16px 0;">
        <span>共 {{ tracks.length }} 首曲目</span>
        <n-button type="primary" @click="handleDownloadAll" :loading="loading">
          全部下载
        </n-button>
      </div>
      <n-data-table :columns="columns" :data="tracks" :max-height="400" size="small" />
    </template>

    <n-empty v-else description="粘贴歌单链接或上传文件开始导入" style="margin-top: 32px" />
  </div>
</template>

<style scoped>
.playlist-page {
  max-width: 800px;
  margin: 0 auto;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 16px;
}

.import-row {
  display: flex;
  gap: 12px;
  margin: 16px 0;
}

.upload-row {
  margin-top: 12px;
}
</style>
