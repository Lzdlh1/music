<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NInput, NDataTable, NButton, NEmpty, NIcon, NSpin, NTag, useMessage } from 'naive-ui'
import { Icon } from '@iconify/vue'
import type { LibraryItem, PlayTrack } from '@/types'
import { listLibrary, libraryStreamUrl, deleteLibraryItem } from '@/api/library'
import { usePlayerStore } from '@/stores/player'
import { useBreakpoint } from '@/composables/useBreakpoint'

const message = useMessage()
const player = usePlayerStore()
const { isMobile } = useBreakpoint()

/** 秒数转 分:秒 */
function fmtDuration(sec: number): string {
  if (!sec || !isFinite(sec) || sec < 0) return '--:--'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

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
        class="library-search"
        clearable
        @update:value="fetchLibrary"
      />
    </div>

    <n-spin :show="loading">
      <!-- 移动端：卡片列表（避免表格横向溢出） -->
      <div v-if="isMobile && items.length" class="mobile-list">
        <div
          v-for="item in items"
          :key="item.id"
          class="mobile-item"
          :class="{ playing: isCurrent(item) }"
        >
          <div class="mi-cover" @click="playOne(item)">
            <img v-if="item.cover_url" :src="item.cover_url" alt="" loading="lazy" />
            <n-icon v-else :size="20" color="#999">
              <Icon icon="material-symbols:music-note" />
            </n-icon>
          </div>
          <div class="mi-info" @click="playOne(item)">
            <div class="mi-title" :title="item.title">{{ item.title }}</div>
            <div class="mi-artist">
              {{ item.artist || '未知艺术家' }}
              <span v-if="item.album"> · {{ item.album }}</span>
            </div>
            <div class="mi-meta">
              <n-tag v-if="item.quality" size="tiny" :bordered="false" round>{{ item.quality }}</n-tag>
              <n-tag v-if="item.format" size="tiny" :bordered="false" round>{{ item.format }}</n-tag>
              <span class="mi-duration">{{ fmtDuration(item.duration) }}</span>
            </div>
          </div>
          <div class="mi-actions">
            <n-button quaternary circle size="small" type="primary" @click="playOne(item)">
              <template #icon>
                <n-icon>
                  <Icon
                    :icon="isCurrent(item) && player.playing ? 'material-symbols:pause-rounded' : 'material-symbols:play-arrow-rounded'"
                    :width="22"
                  />
                </n-icon>
              </template>
            </n-button>
            <n-button quaternary circle size="small" type="error" @click="handleDelete(item)">
              <template #icon>
                <n-icon><Icon icon="material-symbols:delete-outline" :width="18" /></n-icon>
              </template>
            </n-button>
          </div>
        </div>
      </div>

      <!-- 桌面端：数据表格 -->
      <n-data-table
        v-else-if="!isMobile && items.length"
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
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 20px;
}

.library-search {
  width: 300px;
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

/* ---------- 移动端卡片列表 ---------- */
.mobile-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mobile-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--n-card-color, #fff);
  border-radius: 12px;
  border: 1px solid var(--n-border-color, #e0e0e0);
  transition: all 0.2s;
}

.mobile-item.playing {
  border-color: #6366f1;
  background: rgba(99, 102, 241, 0.06);
}

.mi-cover {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
  background: var(--n-fill-color-2, #f5f5f7);
  display: flex;
  align-items: center;
  justify-content: center;
}

.mi-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mi-info {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}

.mi-title {
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mi-artist {
  font-size: 12px;
  color: var(--n-text-color-3, #999);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.mi-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  font-size: 11px;
  overflow: hidden;
}

.mi-duration {
  color: var(--n-text-color-3, #aaa);
  font-variant-numeric: tabular-nums;
}

.mi-actions {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

/* 移动端适配：header 纵向堆叠 + 加大底部留白避免被播放条/标签栏遮挡 */
@media (max-width: 767px) {
  .library-page {
    padding-bottom: 150px;
  }

  .library-header {
    flex-direction: column;
    align-items: stretch;
  }

  .library-search {
    width: 100%;
  }

  .page-title {
    font-size: 20px;
  }
}
</style>
