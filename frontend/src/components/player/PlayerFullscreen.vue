<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { NIcon, NSlider, NSelect, NSwitch } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { useWindowSize } from '@vueuse/core'
import { usePlayerStore, EQ_BANDS, EQ_PRESETS } from '@/stores/player'
import type { LoopMode } from '@/types'

const player = usePlayerStore()
const { width } = useWindowSize()

/** PC 宽屏双栏视图 */
const isWide = computed(() => width.value >= 1024)
/** 移动端视图：封面 / 歌词全屏 */
const view = ref<'cover' | 'lyrics'>('cover')

const current = computed(() => player.currentTrack)
const liked = ref(false)

const coverScrollRef = ref<HTMLElement | null>(null)
const lyricsScrollRef = ref<HTMLElement | null>(null)

function fmt(sec: number) {
  if (!isFinite(sec) || sec < 0) return '00:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const loopIcon = computed(() => {
  switch (player.loopMode) {
    case 'all': return 'material-symbols:repeat'
    case 'one': return 'material-symbols:repeat-one'
    default: return 'material-symbols:repeat-off'
  }
})

const loopLabel = computed(() => {
  switch (player.loopMode) {
    case 'all': return '列表循环'
    case 'one': return '单曲循环'
    default: return '顺序播放'
  }
})

function cycleLoop() {
  const order: LoopMode[] = ['off', 'all', 'one']
  const idx = order.indexOf(player.loopMode)
  player.setLoopMode(order[(idx + 1) % order.length])
}

function toggleLike() {
  liked.value = !liked.value
}

function goLyrics() {
  view.value = 'lyrics'
}

function goCover() {
  view.value = 'cover'
}

/** 当前歌词滚动容器（仅歌词视图/PC歌词栏滚动，封面视图不滚，避免顶部/底部被移出） */
function activeScroll() {
  if (isWide.value) return lyricsScrollRef.value
  return view.value === 'lyrics' ? lyricsScrollRef.value : null
}

// 歌词自动滚动到当前行（只滚容器本身，不触发整页/浏览器滚动）
watch(
  () => player.activeLyricsIndex,
  async (idx) => {
    if (idx < 0) return
    await nextTick()
    const box = activeScroll()
    if (!box) return
    const el = box.querySelector(`.pf-lyric-line[data-idx="${idx}"]`) as HTMLElement | null
    if (el) {
      box.scrollTo({ top: el.offsetTop - box.clientHeight / 2 + el.clientHeight / 2, behavior: 'smooth' })
    }
  },
)

watch(
  () => [player.showFullPlayer, width.value],
  ([open]) => {
    if (open) {
      document.body.style.overflow = 'hidden'
      view.value = 'cover'
    } else {
      document.body.style.overflow = ''
      player.showSettings = false
    }
  },
  { immediate: true },
)
</script>

<template>
  <transition name="pf-slide">
    <div
      v-if="player.showFullPlayer && current"
      class="pf-overlay"
      @click.self="player.showSettings = false"
    >
      <div class="pf-bg">
        <img v-if="current.cover_url" :src="current.cover_url" alt="" />
      </div>

      <!-- ===== PC 宽屏双栏 ===== -->
      <div v-if="isWide" class="pf-wide">
        <!-- 左：封面 + 信息 + 控制 -->
        <div class="pf-wide-left">
          <div class="pf-topbar">
            <button class="ctl ctl-ghost" @click="player.closeFullPlayer()">
              <Icon icon="material-symbols:keyboard-arrow-down-rounded" :width="26" />
            </button>
            <button class="ctl ctl-ghost" @click="player.toggleLyrics()">
              <Icon icon="material-symbols:playlist-play" :width="20" />
            </button>
          </div>

          <div class="pf-cover-wrap">
            <div class="pf-cover">
              <img v-if="current.cover_url" :src="current.cover_url" alt="cover" />
              <n-icon v-else :size="90" color="#888">
                <Icon icon="material-symbols:music-note" />
              </n-icon>
            </div>
          </div>

          <div class="pf-info">
            <div class="pf-title-row">
              <div class="pf-title-box">
                <div class="pf-title">{{ current.title }}</div>
                <div class="pf-artist">
                  {{ current.artist || '未知艺术家' }}
                  <span v-if="current.album"> · {{ current.album }}</span>
                </div>
              </div>
              <button class="ctl" :class="{ 'ctl-liked': liked }" @click="toggleLike">
                <Icon :icon="liked ? 'material-symbols:favorite' : 'material-symbols:favorite-outline'" :width="24" />
              </button>
            </div>
          </div>

          <div class="pf-progress">
            <n-slider
              :value="player.progress"
              :tooltip="false"
              @update:value="player.seekByPercent"
            />
            <div class="pf-times">
              <span>{{ fmt(player.currentTime) }}</span>
              <span>{{ fmt(player.duration) }}</span>
            </div>
          </div>

          <div class="pf-controls">
            <button class="ctl" :class="{ active: player.loopMode !== 'off' }" @click="cycleLoop" :title="loopLabel">
              <Icon :icon="loopIcon" :width="22" />
            </button>
            <button class="ctl ctl-lg" @click="player.prev()">
              <Icon icon="material-symbols:skip-previous-rounded" :width="30" />
            </button>
            <button class="ctl ctl-play" @click="player.toggle()">
              <Icon
                :icon="player.playing ? 'material-symbols:pause-rounded' : 'material-symbols:play-arrow-rounded'"
                :width="30"
              />
            </button>
            <button class="ctl ctl-lg" @click="player.next()">
              <Icon icon="material-symbols:skip-next-rounded" :width="30" />
            </button>
            <button class="ctl" :class="{ active: player.shuffle }" @click="player.toggleShuffle()">
              <Icon icon="material-symbols:shuffle" :width="22" />
            </button>
          </div>

          <div class="pf-wide-actions">
            <span class="pf-quality">HQ</span>
            <button class="ctl" @click="player.toggleSettings()">
              <Icon icon="material-symbols:tune" :width="22" />
            </button>
          </div>
        </div>

        <!-- 右：歌词 -->
        <div class="pf-wide-right">
          <div class="pf-lyrics-hint">歌词</div>
          <div class="pf-lyrics pf-lyrics-wide" ref="lyricsScrollRef">
            <div v-if="player.lyrics.length" class="pf-lyrics-list">
              <div
                v-for="(line, i) in player.lyrics"
                :key="i"
                :data-idx="i"
                class="pf-lyric-line"
                :class="{ active: i === player.activeLyricsIndex }"
                @click="player.seek(line.time / 1000)"
              >
                {{ line.text }}
              </div>
            </div>
            <div v-else-if="player.lyricsLoading" class="pf-lyrics-empty">歌词加载中...</div>
            <div v-else class="pf-lyrics-empty">暂无歌词</div>
          </div>
        </div>
      </div>

      <!-- ===== 移动端：纵向 ===== -->
      <div v-else class="pf-mobile">
        <!-- 封面视图 -->
        <div v-if="view === 'cover'" class="pf-scroll" ref="coverScrollRef">
          <div class="pf-topbar">
            <button class="ctl ctl-ghost" @click="player.closeFullPlayer()">
              <Icon icon="material-symbols:keyboard-arrow-down-rounded" :width="26" />
            </button>
            <button class="ctl ctl-ghost" @click="goLyrics()">
              <Icon icon="material-symbols:playlist-play" :width="20" />
            </button>
          </div>

          <div class="pf-cover-wrap">
            <div class="pf-cover">
              <img v-if="current.cover_url" :src="current.cover_url" alt="cover" />
              <n-icon v-else :size="80" color="#888">
                <Icon icon="material-symbols:music-note" />
              </n-icon>
            </div>
          </div>

          <div class="pf-info">
            <div class="pf-title-row">
              <div class="pf-title-box">
                <div class="pf-title">{{ current.title }}</div>
                <div class="pf-artist">
                  {{ current.artist || '未知艺术家' }}
                  <span v-if="current.album"> · {{ current.album }}</span>
                </div>
              </div>
              <button class="ctl" :class="{ 'ctl-liked': liked }" @click="toggleLike">
                <Icon :icon="liked ? 'material-symbols:favorite' : 'material-symbols:favorite-outline'" :width="24" />
              </button>
            </div>
          </div>

          <!-- 歌词预览（点击进入歌词全屏） -->
          <div class="pf-lyrics pf-lyrics-cover" @click="goLyrics()">
            <div v-if="player.lyrics.length" class="pf-lyrics-list">
              <div
                v-for="(line, i) in player.lyrics"
                :key="i"
                :data-idx="i"
                class="pf-lyric-line"
                :class="{ active: i === player.activeLyricsIndex }"
              >
                {{ line.text }}
              </div>
            </div>
            <div v-else-if="player.lyricsLoading" class="pf-lyrics-empty">歌词加载中...</div>
            <div v-else class="pf-lyrics-empty">暂无歌词</div>
          </div>

          <div class="pf-actions">
            <span class="pf-quality">HQ</span>
            <button class="ctl" @click="player.toggleSettings()">
              <Icon icon="material-symbols:tune" :width="22" />
            </button>
            <button class="ctl" @click="goLyrics()">
              <Icon icon="material-symbols:lyrics-outline" :width="22" />
            </button>
          </div>

          <div class="pf-progress">
            <n-slider
              :value="player.progress"
              :tooltip="false"
              @update:value="player.seekByPercent"
            />
            <div class="pf-times">
              <span>{{ fmt(player.currentTime) }}</span>
              <span>{{ fmt(player.duration) }}</span>
            </div>
          </div>

          <div class="pf-controls">
            <button class="ctl" :class="{ active: player.loopMode !== 'off' }" @click="cycleLoop" :title="loopLabel">
              <Icon :icon="loopIcon" :width="22" />
            </button>
            <button class="ctl ctl-lg" @click="player.prev()">
              <Icon icon="material-symbols:skip-previous-rounded" :width="30" />
            </button>
            <button class="ctl ctl-play" @click="player.toggle()">
              <Icon
                :icon="player.playing ? 'material-symbols:pause-rounded' : 'material-symbols:play-arrow-rounded'"
                :width="30"
              />
            </button>
            <button class="ctl ctl-lg" @click="player.next()">
              <Icon icon="material-symbols:skip-next-rounded" :width="30" />
            </button>
            <button class="ctl" :class="{ active: player.shuffle }" @click="player.toggleShuffle()">
              <Icon icon="material-symbols:shuffle" :width="22" />
            </button>
          </div>
        </div>

        <!-- 歌词全屏视图 -->
        <div v-else class="pf-lyrics-view">
          <div class="pf-lyrics-topbar">
            <button class="ctl ctl-ghost" @click="player.closeFullPlayer()">
              <Icon icon="material-symbols:keyboard-arrow-down-rounded" :width="26" />
            </button>
            <div class="pf-lyrics-title">{{ current.title }}</div>
            <button class="ctl" :class="{ 'ctl-liked': liked }" @click="toggleLike">
              <Icon :icon="liked ? 'material-symbols:favorite' : 'material-symbols:favorite-outline'" :width="24" />
            </button>
            <button class="ctl ctl-liked" @click="goCover()">
              <Icon icon="material-symbols:lyrics-outline" :width="24" />
            </button>
          </div>

          <div class="pf-lyrics pf-lyrics-full" ref="lyricsScrollRef">
            <div v-if="player.lyrics.length" class="pf-lyrics-list">
              <div
                v-for="(line, i) in player.lyrics"
                :key="i"
                :data-idx="i"
                class="pf-lyric-line"
                :class="{ active: i === player.activeLyricsIndex }"
                @click="player.seek(line.time / 1000)"
              >
                {{ line.text }}
              </div>
            </div>
            <div v-else-if="player.lyricsLoading" class="pf-lyrics-empty">歌词加载中...</div>
            <div v-else class="pf-lyrics-empty">暂无歌词</div>
          </div>

          <div class="pf-lyrics-controls">
            <div class="pf-progress">
              <n-slider :value="player.progress" :tooltip="false" @update:value="player.seekByPercent" />
              <div class="pf-times">
                <span>{{ fmt(player.currentTime) }}</span>
                <span>{{ fmt(player.duration) }}</span>
              </div>
            </div>
            <div class="pf-controls">
              <button class="ctl" :class="{ active: player.loopMode !== 'off' }" @click="cycleLoop" :title="loopLabel">
                <Icon :icon="loopIcon" :width="22" />
              </button>
              <button class="ctl ctl-lg" @click="player.prev()">
                <Icon icon="material-symbols:skip-previous-rounded" :width="30" />
              </button>
              <button class="ctl ctl-play" @click="player.toggle()">
                <Icon
                  :icon="player.playing ? 'material-symbols:pause-rounded' : 'material-symbols:play-arrow-rounded'"
                  :width="30"
                />
              </button>
              <button class="ctl ctl-lg" @click="player.next()">
                <Icon icon="material-symbols:skip-next-rounded" :width="30" />
              </button>
              <button class="ctl" :class="{ active: player.shuffle }" @click="player.toggleShuffle()">
                <Icon icon="material-symbols:shuffle" :width="22" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- ===== 设置浮层（EQ/音效） ===== -->
      <div v-if="player.showSettings" class="pf-settings" @click.stop>
        <div class="pf-settings-title">播放设置</div>
        <div class="ps-row">
          <span class="ps-label">播放速度</span>
          <n-select
            size="small"
            style="width: 90px"
            :value="player.playbackRate"
            :options="[0.5, 0.75, 1, 1.25, 1.5, 2].map((r) => ({ label: `${r}x`, value: r }))"
            @update:value="player.setPlaybackRate"
          />
        </div>
        <div class="ps-row">
          <span class="ps-label">均衡器</span>
          <n-switch
            :value="player.eqEnabled"
            @update:value="(v: boolean) => player.setEqEnabled(v)"
          />
        </div>
        <div class="ps-row">
          <span class="ps-label">音量</span>
          <n-slider
            :min="0"
            :max="100"
            :value="player.volumePercent"
            @update:value="(v: number) => player.setVolume(v / 100)"
          />
          <span class="ps-vol">{{ player.volumePercent }}%</span>
        </div>
        <template v-if="player.eqEnabled">
          <div class="ps-presets">
            <button
              v-for="p in EQ_PRESETS"
              :key="p.name"
              class="ps-chip"
              :class="{ active: player.eqPreset === p.name }"
              @click="player.setEqPreset(p.name)"
            >
              {{ p.name }}
            </button>
          </div>
          <div class="ps-eq">
            <div v-for="(freq, i) in EQ_BANDS" :key="freq" class="ps-eq-item">
              <span class="ps-eq-freq">{{ freq >= 1000 ? `${freq / 1000}k` : freq }}</span>
              <n-slider
                vertical
                :min="-12"
                :max="12"
                :step="1"
                :value="player.eqGains[i]"
                :tooltip="false"
                class="ps-eq-slider"
                @update:value="(v: number) => {
                  const g = [...player.eqGains]
                  g[i] = v
                  player.setEqGains(g)
                }"
              />
              <span class="ps-eq-gain">{{ player.eqGains[i] }}</span>
            </div>
          </div>
        </template>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.pf-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: #1c1b22;
  color: #fff;
  overflow: hidden;
  display: flex;
}

.pf-bg {
  position: absolute;
  inset: 0;
  background: linear-gradient(165deg, #4a3f52 0%, #201d28 60%, #17151f 100%);
}

.pf-bg img {
  position: absolute;
  inset: -60px;
  width: calc(100% + 120px);
  height: calc(100% + 120px);
  object-fit: cover;
  opacity: 0.16;
  filter: blur(50px) saturate(1.5);
}

/* ---------- 通用圆角按钮，对比明显 ---------- */
.ctl {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 22px;
  border: none;
  background: rgba(255, 255, 255, 0.16);
  color: rgba(255, 255, 255, 0.96);
  cursor: pointer;
  transition: background 0.2s, transform 0.1s;
}

.ctl:hover {
  background: rgba(255, 255, 255, 0.26);
}

.ctl:active {
  transform: scale(0.94);
}

.ctl.active {
  background: rgba(255, 122, 158, 0.9);
  color: #fff;
}

.ctl-ghost {
  background: transparent;
  color: rgba(255, 255, 255, 0.92);
}

.ctl-ghost:hover {
  background: rgba(255, 255, 255, 0.14);
}

.ctl-lg {
  width: 56px;
  height: 56px;
}

.ctl-play {
  width: 70px;
  height: 70px;
  background: #fff;
  color: #17151f;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.35);
}

.ctl-play:hover {
  background: #f4f4f6;
}

.ctl-liked {
  color: #ff5d73;
}

/* ---------- 顶栏 ---------- */
.pf-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

/* ---------- 封面 ---------- */
.pf-cover-wrap {
  display: flex;
  justify-content: center;
  margin: 10px 0 0;
}

.pf-cover {
  width: 100%;
  max-width: 280px;
  aspect-ratio: 1;
  border-radius: 18px;
  overflow: hidden;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.45);
  background: #26232c;
  display: flex;
  align-items: center;
  justify-content: center;
}

.pf-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* ---------- 标题信息 ---------- */
.pf-info {
  margin: 22px 4px 0;
}

.pf-title-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.pf-title-box {
  flex: 1;
  min-width: 0;
}

.pf-title {
  font-size: 22px;
  font-weight: 700;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pf-artist {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.66);
  margin-top: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ---------- 歌词 ---------- */
.pf-lyrics {
  margin: 14px 0;
  mask-image: linear-gradient(to bottom, transparent 0, #000 16%, #000 84%, transparent 100%);
  -webkit-mask-image: linear-gradient(to bottom, transparent 0, #000 16%, #000 84%, transparent 100%);
}

.pf-lyrics-cover {
  max-height: 132px;
  overflow: hidden;
  cursor: pointer;
}

.pf-lyrics-list {
  padding: 18px 0;
}

.pf-lyric-line {
  padding: 8px 0;
  font-size: 15px;
  line-height: 1.6;
  color: rgba(255, 255, 255, 0.52);
  text-align: center;
  cursor: pointer;
  transition: color 0.2s;
}

.pf-lyric-line.active {
  color: #fff;
  font-weight: 700;
}

.pf-lyrics-empty {
  text-align: center;
  color: rgba(255, 255, 255, 0.4);
  padding: 40px 0;
  font-size: 14px;
}

/* ---------- 功能图标行 ---------- */
.pf-actions {
  display: flex;
  align-items: center;
  gap: 14px;
  margin: 4px 4px 12px;
}

.pf-quality {
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
  padding: 4px 10px;
  border-radius: 12px;
}

/* ---------- 进度 ---------- */
.pf-progress {
  margin: 0 4px 12px;
}

.pf-times {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
  margin-top: 6px;
  font-variant-numeric: tabular-nums;
}

/* ---------- 控制按钮 ---------- */
.pf-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4px;
}

/* ---------- 移动端容器 ---------- */
.pf-mobile {
  position: relative;
  width: 100%;
  max-width: 480px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  height: 100%;
}

.pf-scroll {
  flex: 1;
  overflow-y: auto;
  padding: calc(12px + env(safe-area-inset-top)) 22px calc(20px + env(safe-area-inset-bottom));
  -webkit-overflow-scrolling: touch;
}

/* 手机端：封面缩小、间距收紧，让封面+标题+歌词预览+控制一屏内同时出现 */
.pf-mobile .pf-cover {
  max-width: 236px;
}

.pf-mobile .pf-cover-wrap {
  margin: 6px 0 0;
}

.pf-mobile .pf-info {
  margin: 14px 4px 0;
}

.pf-mobile .pf-lyrics-cover {
  max-height: 100px;
}

.pf-mobile .pf-actions {
  margin: 0 4px 8px;
}

.pf-mobile .pf-progress {
  margin: 0 4px 8px;
}

.pf-mobile .pf-controls {
  padding: 0 2px;
}

/* ---------- 歌词全屏视图 ---------- */
.pf-lyrics-view {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.pf-lyrics-topbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: calc(12px + env(safe-area-inset-top)) 18px 12px;
  justify-content: space-between;
}

.pf-lyrics-title {
  flex: 1;
  min-width: 0;
  text-align: center;
  font-size: 14px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.88);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pf-mini {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.pf-mini-cover {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  overflow: hidden;
  flex-shrink: 0;
  background: #26232c;
  display: flex;
  align-items: center;
  justify-content: center;
}

.pf-mini-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.pf-mini-meta {
  min-width: 0;
}

.pf-mini-title {
  font-size: 15px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pf-mini-artist {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.pf-lyrics-full {
  position: relative;
  flex: 1;
  overflow-y: auto;
  margin: 0;
  padding: 0 24px;
  -webkit-overflow-scrolling: touch;
  mask-image: linear-gradient(to bottom, transparent 0, #000 12%, #000 88%, transparent 100%);
  -webkit-mask-image: linear-gradient(to bottom, transparent 0, #000 12%, #000 88%, transparent 100%);
}

.pf-lyrics-full .pf-lyric-line {
  font-size: 17px;
  padding: 9px 0;
}

.pf-lyrics-full .pf-lyric-line.active {
  color: #fff;
}

.pf-lyrics-controls {
  padding: 12px 24px calc(24px + env(safe-area-inset-bottom));
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.pf-tap-hint {
  display: block;
  text-align: center;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  margin-bottom: 8px;
}

/* ---------- PC 双栏 ---------- */
.pf-wide {
  position: relative;
  width: 100%;
  max-width: 1080px;
  margin: 0 auto;
  display: flex;
  height: 100%;
  padding: 0 48px;
}

.pf-wide-left {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  padding: 24px 40px 24px 0;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.pf-wide-left .pf-cover {
  max-width: 380px;
}

.pf-wide-left .pf-controls {
  max-width: 380px;
  margin-top: 8px;
}

.pf-wide-actions {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 20px;
  max-width: 380px;
}

.pf-wide-right {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 24px 0 24px 40px;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}

.pf-lyrics-hint {
  font-size: 16px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.85);
  margin-bottom: 8px;
}

.pf-lyrics-wide {
  position: relative;
  flex: 1;
  overflow-y: auto;
  margin: 0;
  -webkit-overflow-scrolling: touch;
  mask-image: none;
  -webkit-mask-image: none;
  text-align: center;
}

/* ---------- 设置浮层 ---------- */
.pf-settings {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(28, 27, 34, 0.98);
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  padding: 22px 24px calc(24px + env(safe-area-inset-bottom));
  z-index: 10;
  max-height: 72vh;
  overflow-y: auto;
  backdrop-filter: blur(12px);
}

.pf-settings-title {
  font-size: 16px;
  font-weight: 700;
  margin-bottom: 14px;
}

.ps-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
}

.ps-label {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.72);
  width: 60px;
  flex-shrink: 0;
}

.ps-vol {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
  width: 40px;
  text-align: right;
}

.ps-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 0;
}

.ps-chip {
  border: none;
  border-radius: 14px;
  padding: 5px 12px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.8);
  background: rgba(255, 255, 255, 0.12);
  cursor: pointer;
}

.ps-chip.active {
  background: rgba(255, 122, 158, 0.9);
  color: #fff;
}

.ps-eq {
  display: flex;
  gap: 12px;
  padding: 14px 0;
  justify-content: space-between;
}

.ps-eq-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.ps-eq-slider {
  height: 100px;
}

.ps-eq-freq {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
}

.ps-eq-gain {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.5);
}

/* ---------- 无感滚动条（保留滚轮/触摸滚动，隐藏滚动条） ---------- */
.pf-scroll,
.pf-lyrics-full,
.pf-lyrics-wide,
.pf-wide-left,
.pf-settings {
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.pf-scroll::-webkit-scrollbar,
.pf-lyrics-full::-webkit-scrollbar,
.pf-lyrics-wide::-webkit-scrollbar,
.pf-wide-left::-webkit-scrollbar,
.pf-settings::-webkit-scrollbar {
  display: none;
  width: 0;
  height: 0;
}

/* ---------- 动画 ---------- */
.pf-slide-enter-active,
.pf-slide-leave-active {
  transition: transform 0.32s cubic-bezier(0.22, 1, 0.36, 1);
}

.pf-slide-enter-from,
.pf-slide-leave-to {
  transform: translateY(100%);
}
</style>
