<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { NSlider, NPopover, NIcon, NButton, NSwitch, NSelect, NInputNumber, NEmpty, NScrollbar } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { usePlayerStore, EQ_BANDS, EQ_PRESETS } from '@/stores/player'
import type { LoopMode } from '@/types'

const player = usePlayerStore()

const dragging = ref(false)
const dragValue = ref(0)

const current = computed(() => player.currentTrack)

function fmt(sec: number) {
  if (!isFinite(sec) || sec < 0) return '00:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const loopIcon = computed(() => {
  switch (player.loopMode) {
    case 'all':
      return 'material-symbols:repeat'
    case 'one':
      return 'material-symbols:repeat-one'
    default:
      return 'material-symbols:repeat-off'
  }
})

const loopLabel = computed(() => {
  switch (player.loopMode) {
    case 'all':
      return '列表循环'
    case 'one':
      return '单曲循环'
    default:
      return '顺序播放'
  }
})

const playbackRateOptions = [0.5, 0.75, 1, 1.25, 1.5, 2].map((r) => ({
  label: `${r}x`,
  value: r,
}))

function onProgressChange(v: number) {
  dragging.value = false
  player.seekByPercent(v)
}

let raf = 0
function tick() {
  if (!dragging.value) {
    dragValue.value = player.progress
  }
  raf = requestAnimationFrame(tick)
}

onMounted(() => {
  tick()
})
onUnmounted(() => {
  cancelAnimationFrame(raf)
})

watch(
  () => player.currentIndex,
  () => {
    if (current.value) {
      player.loadLyricsFor(current.value)
    }
  },
)
</script>

<template>
  <div class="player-bar" v-if="current">
    <!-- 左侧：封面 + 信息 -->
    <div class="pb-left" @click="player.toggleLyrics()">
      <div class="cover">
        <img v-if="current.cover_url" :src="current.cover_url" alt="" />
        <Icon v-else icon="material-symbols:music-note" :width="26" class="cover-fallback" />
      </div>
      <div class="meta">
        <div class="title" :title="current.title">{{ current.title }}</div>
        <div class="artist" :title="current.artist">{{ current.artist || '未知艺术家' }}</div>
      </div>
    </div>

    <!-- 中间：控制 + 进度 -->
    <div class="pb-center">
      <div class="controls">
        <n-button quaternary circle size="small" @click="player.setLoopMode((player.loopMode === 'off' ? 'all' : player.loopMode === 'all' ? 'one' : 'off') as LoopMode)">
          <template #icon>
            <n-icon><Icon :icon="loopIcon" :width="18" /></n-icon>
          </template>
        </n-button>
        <n-button quaternary circle @click="player.prev()">
          <template #icon>
            <n-icon size="22"><Icon icon="material-symbols:skip-previous-rounded" :width="24" /></n-icon>
          </template>
        </n-button>
        <n-button circle type="primary" class="play-btn" @click="player.toggle()">
          <template #icon>
            <n-icon size="26">
              <Icon v-if="player.playing" icon="material-symbols:pause-rounded" :width="26" />
              <Icon v-else icon="material-symbols:play-arrow-rounded" :width="26" />
            </n-icon>
          </template>
        </n-button>
        <n-button quaternary circle @click="player.next()">
          <template #icon>
            <n-icon size="22"><Icon icon="material-symbols:skip-next-rounded" :width="24" /></n-icon>
          </template>
        </n-button>
        <n-button quaternary circle :type="player.shuffle ? 'primary' : 'default'" @click="player.toggleShuffle()">
          <template #icon>
            <n-icon><Icon icon="material-symbols:shuffle" :width="18" /></n-icon>
          </template>
        </n-button>
      </div>
      <div class="progress-row">
        <span class="time">{{ fmt(player.currentTime) }}</span>
        <n-slider
          :value="dragging ? dragValue : player.progress"
          :tooltip="false"
          class="progress-slider"
          @update:value="dragValue = $event"
          @update:value:start="dragging = true"
          @update:value:end="onProgressChange"
        />
        <span class="time">{{ fmt(player.duration) }}</span>
      </div>
    </div>

    <!-- 右侧：音效/歌词/音量/设置 -->
    <div class="pb-right">
      <n-popover trigger="click" placement="top-end" :show="player.showLyrics" @update:show="(v: boolean) => (player.showLyrics = v)">
        <template #trigger>
          <n-button quaternary circle :type="player.showLyrics ? 'primary' : 'default'" @click="player.toggleLyrics()">
            <template #icon>
              <n-icon><Icon icon="material-symbols:lyrics-outline" :width="20" /></n-icon>
            </template>
          </n-button>
        </template>
        <div class="lyrics-panel">
          <div class="panel-title">歌词</div>
          <n-scrollbar style="max-height: 340px">
            <div v-if="player.lyrics.length" class="lyrics-list">
              <div
                v-for="(line, i) in player.lyrics"
                :key="i"
                class="lyric-line"
                :class="{ active: i === player.activeLyricsIndex }"
                @click="player.seek(line.time / 1000)"
              >
                {{ line.text }}
              </div>
            </div>
            <n-empty v-else-if="!player.lyricsLoading" description="暂无歌词" size="small" />
            <div v-else class="lyrics-loading">歌词加载中...</div>
          </n-scrollbar>
        </div>
      </n-popover>

      <n-popover trigger="click" placement="top-end" :show="player.showSettings" @update:show="(v: boolean) => (player.showSettings = v)">
        <template #trigger>
          <n-button quaternary circle :type="player.showSettings ? 'primary' : 'default'" @click="player.toggleSettings()">
            <template #icon>
              <n-icon><Icon icon="material-symbols:tune" :width="20" /></n-icon>
            </template>
          </n-button>
        </template>
        <div class="settings-panel">
          <div class="panel-title">播放设置</div>

          <div class="setting-row">
            <span class="label">播放速度</span>
            <n-select
              size="small"
              style="width: 90px"
              :value="player.playbackRate"
              :options="playbackRateOptions"
              @update:value="player.setPlaybackRate"
            />
          </div>

          <div class="setting-row">
            <span class="label">均衡器</span>
            <n-switch :value="player.eqEnabled" @update:value="player.setEqEnabled" />
          </div>

          <template v-if="player.eqEnabled">
            <div class="eq-presets">
              <n-button
                v-for="p in EQ_PRESETS"
                :key="p.name"
                size="tiny"
                :type="player.eqPreset === p.name ? 'primary' : 'default'"
                @click="player.setEqPreset(p.name)"
              >
                {{ p.name }}
              </n-button>
            </div>
            <div class="eq-sliders">
              <div v-for="(freq, i) in EQ_BANDS" :key="freq" class="eq-slider">
                <span class="eq-freq">{{ freq >= 1000 ? `${freq / 1000}k` : freq }}</span>
                <n-slider
                  vertical
                  :min="-12"
                  :max="12"
                  :step="1"
                  :value="player.eqGains[i]"
                  :tooltip="false"
                  class="eq-slider-bar"
                  @update:value="(v: number) => {
                    const gains = [...player.eqGains]
                    gains[i] = v
                    player.setEqGains(gains)
                  }"
                />
                <span class="eq-gain">{{ player.eqGains[i] > 0 ? `+${player.eqGains[i]}` : player.eqGains[i] }}</span>
              </div>
            </div>
          </template>

          <div class="setting-row">
            <span class="label">音量</span>
            <n-slider
              :min="0"
              :max="100"
              :value="player.volumePercent"
              class="volume-slider"
              @update:value="player.setVolume($event / 100)"
            />
            <span class="volume-num">{{ player.volumePercent }}%</span>
          </div>

          <div class="setting-row">
            <span class="label">播放模式</span>
            <span class="loop-label">{{ loopLabel }}</span>
          </div>
        </div>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.player-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 72px;
  background: rgba(24, 24, 28, 0.96);
  backdrop-filter: blur(12px);
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  align-items: center;
  padding: 0 20px;
  gap: 24px;
  z-index: 1000;
  color: #fff;
}

.pb-left {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 240px;
  min-width: 180px;
  cursor: pointer;
  overflow: hidden;
}

.cover {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  background: #2a2a31;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-fallback {
  color: #999;
}

.meta {
  overflow: hidden;
}

.title {
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.artist {
  font-size: 12px;
  color: #aaa;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.pb-center {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.controls {
  display: flex;
  align-items: center;
  gap: 6px;
}

.play-btn {
  width: 40px;
  height: 40px;
}

.progress-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  max-width: 720px;
}

.time {
  font-size: 11px;
  color: #aaa;
  font-variant-numeric: tabular-nums;
  min-width: 36px;
  text-align: center;
}

.progress-slider {
  flex: 1;
}

.pb-right {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 120px;
  justify-content: flex-end;
}

/* 歌词面板 */
.lyrics-panel {
  width: 320px;
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 10px;
}

.lyrics-list {
  padding: 4px 0;
}

.lyric-line {
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  color: #ccc;
  font-size: 13px;
  line-height: 1.5;
  transition: all 0.2s;
}

.lyric-line:hover {
  background: rgba(255, 255, 255, 0.06);
}

.lyric-line.active {
  color: #6366f1;
  font-weight: 600;
}

.lyrics-loading {
  padding: 20px;
  text-align: center;
  color: #999;
}

/* 设置面板 */
.settings-panel {
  width: 340px;
}

.setting-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
}

.label {
  font-size: 13px;
  color: #ccc;
  width: 64px;
  flex-shrink: 0;
}

.loop-label {
  font-size: 12px;
  color: #888;
}

.eq-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px 0;
}

.eq-sliders {
  display: flex;
  gap: 10px;
  padding: 12px 0;
  justify-content: space-between;
}

.eq-slider {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  height: 160px;
}

.eq-slider-bar {
  height: 110px;
}

.eq-freq {
  font-size: 10px;
  color: #999;
}

.eq-gain {
  font-size: 10px;
  color: #666;
  font-variant-numeric: tabular-nums;
}

.volume-slider {
  flex: 1;
}

.volume-num {
  font-size: 12px;
  color: #999;
  width: 40px;
  text-align: right;
}
</style>
