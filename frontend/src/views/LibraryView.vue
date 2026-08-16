<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NInput, NDataTable, NButton, NEmpty, NIcon, NSpin, NTag, useMessage } from 'naive-ui'
import { Icon } from '@iconify/vue'
import type { LibraryItem, PlayTrack } from '@/types'
import { listLibrary, libraryStreamUrl, deleteLibraryItem } from '@/api/library'
import { usePlayerStore } from '@/stores/player'

const message = useMessage()
const player = usePlayerStore()

const items = ref<LibraryItem[]>([])
const total = ref(0)
const loading = ref(false)
const searchQuery = ref('')
const page = ref(1)

/** 将库记录转为播放轨道 */
function toPlayTrack(item: LibraryItem): PlayTrack {
  return {
    id: item.id,
    title: item.title,
    artist: item.artist || '',
    album: item.album || '',
    cover_url: item.cover_url,
    duration: item.duration || 0,
    src: libraryStreamUrl(item.id),
  }
}

function playOne(item: LibraryItem) {
  const queue = items.value.map(toPlayTrack)
  player.playTrack(toPlayTrack(item), queue)
}

function isCurrent(item: LibraryItem) {
  const cur = player.currentTrack
  return cur && cur.id === item.id && !cur.from_cloud
}

async function handleDelete(item: LibraryItem) {
  if (!window.confirm(`确定从音乐库删除 "${item.title}" 吗？`)) return
  try {
    await deleteLibraryItem(item.id)
    message.success('已删除')
    fetchLibrary()
  } catch (e: any) {
    message.error(e.response?.data?.message || '删除失败')
  }
}

const columns = [
  {
    title: '播放',
    key: 'play',
    width: 56,
    render: (row: LibraryItem) =>
      h(
        NButton,
        {
          quaternary: true,
          circle: true,
          type: isCurrent(row) ? 'primary' : 'default',
          onClick: () => playOne(row),
        },
        {
          default: () =>
            h(NIcon, null, {
              default: () =>
                h(Icon, {
                  icon: isCurrent(row) && player.playing
                    ? 'material-symbols:pause-rounded'
                    : 'material-symbols:play-arrow-rounded',
                  width: 20,
                }),
            }),
        },
      ),
  },
  {
    title: '封面',
    key: 'cover',
    width: 56,
    render: (row: LibraryItem) =>
      h('div', { class: 'cover-cell' }, [
        row.cover_url
          ? h('img', { src: row.cover_url, class: 'cover-img' })
          : h(NIcon, { size: 18, color: '#999' }, {
              default: () => h(Icon, { icon: 'material-symbols:music-note' }),
            }),
      ]),
  },
  { title: '歌名', key: 'title', ellipsis: true, minWidth: 180 },
  { title: '艺术家', key: 'artist', width: 160, ellipsis: true, render: (r: LibraryItem) => r.artist || '-' },
  { title: '专辑', key: 'album', width: 200, ellipsis: true, render: (r: LibraryItem) => r.album || '-' },
  { title: '音质', key: 'quality', width: 90, render: (r: LibraryItem) => h(NTag, { size: 'small', bordered: false }, { default: () => r.quality || '-' }) },
  { title: '格式', key: 'format', width: 70, render: (r: LibraryItem) => r.format || '-' },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (row: LibraryItem) =>
      h(NButton, { size: 'tiny', onClick: () => handleDelete(row) }, { default: () => '删除' }),
  },
]

async function fetchLibrary() {
  loading.value = true
  try {
    const res = await listLibrary(searchQuery.value || undefined, page.value, 50)
    items.value = res.data.data || []
    total.value = res.data.total || 0
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载音乐库失败')
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

    <n-spin :show="loading">
      <n-data-table
        v-if="items.length"
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="(row: LibraryItem) => row.id"
        :row-props="(row: LibraryItem) => ({ style: 'cursor:pointer' })"
        striped
        size="small"
        @update:row-key="() => {}"
      />
      <n-empty v-else-if="!loading" description="音乐库为空" style="padding: 60px 0" />
    </n-spin>
  </div>
</template>

<style scoped>
.library-page {
  max-width: 1100px;
  margin: 0 auto;
  padding-bottom: 80px;
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

:global(.cover-cell) {
  display: flex;
  align-items: center;
  justify-content: center;
}

/* render 函数 h() 创建的节点不携带 scoped 属性，需用全局选择器 */
:global(.cover-cell .cover-img) {
  width: 36px;
  height: 36px;
  border-radius: 6px;
  object-fit: cover;
}
</style>
