<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue'
import {
  NSelect,
  NButton,
  NBreadcrumb,
  NBreadcrumbItem,
  NDataTable,
  NEmpty,
  NSpin,
  NModal,
  NInput,
  NUpload,
  NProgress,
  NIcon,
  useMessage,
  type DataTableColumns,
  type UploadCustomRequestOptions,
} from 'naive-ui'
import { Icon } from '@iconify/vue'
import {
  listStorageTargets,
  browseStorage,
  mkdirStorage,
  renameStorage,
  deleteStorageFile,
  uploadStorageFile,
  storageStreamUrl,
  isAudioFile,
  isLyricsFile,
} from '@/api/storage'
import type { StorageTarget, FileInfo } from '@/types'
import { usePlayerStore } from '@/stores/player'
import type { PlayTrack } from '@/types'

const message = useMessage()
const player = usePlayerStore()

const storages = ref<StorageTarget[]>([])
const storageId = ref('')
const currentPath = ref('/')
const files = ref<FileInfo[]>([])
const loading = ref(false)

const showMkdir = ref(false)
const mkdirName = ref('')
const showRename = ref(false)
const renameTarget = ref<FileInfo | null>(null)
const renameNew = ref('')
const uploading = ref(false)
const uploadProgress = ref(0)
const uploadName = ref('')

const audioFiles = computed(() => files.value.filter((f) => !f.is_dir && isAudioFile(f.name)))

const crumbPath = computed(() => {
  const p = currentPath.value === '/' ? '' : currentPath.value
  const segs = p.split('/').filter(Boolean)
  return segs
})

async function loadStorages() {
  const res = await listStorageTargets()
  storages.value = res.data.data || []
  if (!storageId.value && storages.value.length) {
    storageId.value = storages.value[0].id
  }
}

async function loadFiles() {
  if (!storageId.value) {
    files.value = []
    return
  }
  loading.value = true
  try {
    const res = await browseStorage(storageId.value, currentPath.value)
    files.value = (res.data.data || []).sort((a, b) => {
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
      return a.name.localeCompare(b.name)
    })
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载目录失败')
  } finally {
    loading.value = false
  }
}

watch(storageId, () => {
  currentPath.value = '/'
  loadFiles()
})

onMounted(async () => {
  await loadStorages()
  loadFiles()
})

function enterDir(dir: FileInfo) {
  currentPath.value = dir.path
  loadFiles()
}

/** 双击行：文件夹则进入，文件则播放 */
function openRow(row: FileInfo) {
  if (row.is_dir) {
    enterDir(row)
  } else if (isAudioFile(row.name)) {
    playAudio(row)
  }
}

function goCrumb(index: number) {
  const segs = crumbPath.value.slice(0, index + 1)
  currentPath.value = segs.length ? '/' + segs.join('/') : '/'
  loadFiles()
}

function joinPath(name: string) {
  return currentPath.value === '/' ? `/${name}` : `${currentPath.value}/${name}`
}

async function handleMkdir() {
  if (!mkdirName.value.trim()) return
  try {
    await mkdirStorage(storageId.value, joinPath(mkdirName.value.trim()))
    message.success('文件夹已创建')
    showMkdir.value = false
    mkdirName.value = ''
    loadFiles()
  } catch (e: any) {
    message.error(e.response?.data?.message || '创建失败')
  }
}

function openRename(f: FileInfo) {
  renameTarget.value = f
  renameNew.value = f.name
  showRename.value = true
}

async function handleRename() {
  if (!renameTarget.value || !renameNew.value.trim()) return
  try {
    await renameStorage(storageId.value, renameTarget.value.path, joinPath(renameNew.value.trim()))
    message.success('重命名成功')
    showRename.value = false
    loadFiles()
  } catch (e: any) {
    message.error(e.response?.data?.message || '重命名失败')
  }
}

async function handleDelete(f: FileInfo) {
  if (!window.confirm(`确定删除 "${f.name}" 吗？此操作不可恢复。`)) return
  try {
    await deleteStorageFile(storageId.value, f.path)
    message.success('已删除')
    loadFiles()
  } catch (e: any) {
    message.error(e.response?.data?.message || '删除失败')
  }
}

async function handleUpload(options: UploadCustomRequestOptions) {
  const file = options.file.file
  if (!file || !storageId.value) return
  uploading.value = true
  uploadProgress.value = 0
  uploadName.value = file.name
  try {
    await uploadStorageFile(storageId.value, currentPath.value, file, (p) => {
      uploadProgress.value = p
    })
    message.success(`"${file.name}" 上传完成`)
    options.onFinish()
    loadFiles()
  } catch (e: any) {
    message.error(e.response?.data?.message || '上传失败')
    options.onError()
  } finally {
    uploading.value = false
  }
}

/** 下载（浏览器直接打开 stream?download=1） */
function handleDownload(f: FileInfo) {
  const a = document.createElement('a')
  a.href = storageStreamUrl(storageId.value, f.path, true)
  a.download = f.name
  a.click()
}

/** 读取同名 lrc 作为歌词 */
async function fetchLrcFor(audioName: string): Promise<string> {
  const base = audioName.replace(/\.[^.]+$/, '')
  const lrcFile = files.value.find((f) => !f.is_dir && isLyricsFile(f.name) && f.name.replace(/\.[^.]+$/, '') === base)
  if (!lrcFile) return ''
  try {
    const res = await fetch(storageStreamUrl(storageId.value, lrcFile.path))
    if (!res.ok) return ''
    return await res.text()
  } catch {
    return ''
  }
}

/** 读取同目录 cover.jpg 作为封面 */
function fetchCoverFor(audioName: string): string {
  const base = audioName.replace(/\.[^.]+$/, '')
  const cover = files.value.find(
    (f) => !f.is_dir && /^cover\.(jpg|jpeg|png)$/i.test(f.name)
  ) || files.value.find(
    (f) => !f.is_dir && /\.(jpg|jpeg|png)$/i.test(f.name) && f.name.replace(/\.[^.]+$/, '') === base
  )
  return cover ? storageStreamUrl(storageId.value, cover.path) : ''
}

/** 播放：以当前目录所有音频为队列 */
async function playAudio(f: FileInfo) {
  if (!storageId.value) return
  const lrc = await fetchLrcFor(f.name)
  const queue: PlayTrack[] = audioFiles.value.map((af) => ({
    id: `cloud:${storageId.value}:${af.path}`,
    title: af.name.replace(/\.[^.]+$/, ''),
    artist: '',
    album: currentPath.value,
    cover_url: af.path === f.path ? fetchCoverFor(af.name) : '',
    duration: 0,
    src: storageStreamUrl(storageId.value, af.path),
    storage_id: storageId.value,
    path: af.path,
    from_cloud: true,
    lrc: af.path === f.path ? lrc : undefined,
  }))
  const track = queue.find((t) => t.path === f.path) || queue[0]
  await player.playTrack(track, queue)
}

function fmtSize(size: number) {
  if (!size) return '-'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`
}

const columns: DataTableColumns<FileInfo> = [
  {
    title: '名称',
    key: 'name',
    render: (row) =>
      h('div', { class: 'file-name' }, [
        h(Icon, {
          icon: row.is_dir
            ? 'material-symbols:folder'
            : isAudioFile(row.name)
              ? 'material-symbols:music-note'
              : isLyricsFile(row.name)
                ? 'material-symbols:lyrics-outline'
                : 'material-symbols:description-outline',
          width: 18,
          color: row.is_dir ? '#e8b64c' : '#888',
          style: { verticalAlign: '-3px', marginRight: '8px' },
        }),
        row.name,
      ]),
  },
  { title: '大小', key: 'size', width: 120, render: (row) => (row.is_dir ? '-' : fmtSize(row.size)) },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row) =>
      h('div', { style: 'display:flex;gap:6px' }, [
        !row.is_dir
          ? h(
              NButton,
              {
                size: 'tiny',
                type: 'primary',
                disabled: !isAudioFile(row.name),
                onClick: () => playAudio(row),
              },
              { default: () => '播放' },
            )
          : null,
        h(NButton, { size: 'tiny', onClick: () => openRename(row) }, { default: () => '重命名' }),
        row.is_dir
          ? h(NButton, { size: 'tiny', onClick: () => handleDelete(row) }, { default: () => '删除' })
          : h(NButton, { size: 'tiny', onClick: () => handleDownload(row) }, { default: () => '下载' }),
        !row.is_dir
          ? h(NButton, { size: 'tiny', onClick: () => handleDelete(row) }, { default: () => '删除' })
          : null,
      ]),
  },
]
</script>

<template>
  <div class="cloud-page">
    <div class="cloud-header">
      <h1 class="page-title">网盘文件</h1>
      <div class="header-actions">
        <n-select
          v-model:value="storageId"
          placeholder="选择存储后端"
          style="width: 220px"
          :options="storages.map((s) => ({ label: `${s.name} (${s.type})`, value: s.id }))"
        />
        <n-upload
          :show-file-list="false"
          :custom-request="handleUpload"
          :disabled="!storageId || uploading"
        >
          <n-button type="primary" :loading="uploading">
            <template #icon>
              <n-icon><Icon icon="material-symbols:upload" :width="18" /></n-icon>
            </template>
            上传文件
          </n-button>
        </n-upload>
        <n-button @click="showMkdir = true">
          <template #icon>
            <n-icon><Icon icon="material-symbols:create-new-folder" :width="18" /></n-icon>
          </template>
          新建文件夹
        </n-button>
        <n-button @click="loadFiles">
          <template #icon>
            <n-icon><Icon icon="material-symbols:refresh" :width="18" /></n-icon>
          </template>
        </n-button>
      </div>
    </div>

    <div v-if="uploading" class="upload-progress">
      <n-progress type="line" :percentage="uploadProgress" :show-indicator="false" :height="4" />
      <span class="upload-name">正在上传：{{ uploadName }}</span>
    </div>

    <div class="breadcrumb-row">
      <n-breadcrumb>
        <n-breadcrumb-item><a @click.prevent="goCrumb(-1)">根目录</a></n-breadcrumb-item>
        <n-breadcrumb-item v-for="(seg, i) in crumbPath" :key="i">
          <a @click.prevent="goCrumb(i)">{{ seg }}</a>
        </n-breadcrumb-item>
      </n-breadcrumb>
    </div>

    <n-spin :show="loading">
      <n-data-table
        v-if="files.length"
        :columns="columns"
        :data="files"
        :row-key="(row: FileInfo) => row.path"
        :row-props="(row: FileInfo) => ({
          style: 'cursor:' + (row.is_dir ? 'pointer' : 'default'),
          onDblclick: () => openRow(row),
        })"
        striped
        size="small"
      />
      <n-empty v-else description="该目录为空" style="padding: 60px 0" />
    </n-spin>

    <n-modal v-model:show="showMkdir" preset="card" title="新建文件夹" style="width: 360px">
      <n-input v-model:value="mkdirName" placeholder="文件夹名称" @keyup.enter="handleMkdir" />
      <template #footer>
        <n-button type="primary" @click="handleMkdir">创建</n-button>
      </template>
    </n-modal>

    <n-modal v-model:show="showRename" preset="card" title="重命名" style="width: 360px">
      <n-input v-model:value="renameNew" placeholder="新名称" @keyup.enter="handleRename" />
      <template #footer>
        <n-button type="primary" @click="handleRename">确定</n-button>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.cloud-page {
  max-width: 1100px;
  margin: 0 auto;
  padding-bottom: 80px;
}

.cloud-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}

.upload-progress {
  margin-bottom: 12px;
}

.upload-name {
  font-size: 12px;
  color: #999;
}

.breadcrumb-row {
  margin-bottom: 12px;
}
</style>
