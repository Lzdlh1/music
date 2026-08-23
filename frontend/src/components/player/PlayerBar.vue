<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { NIcon, NButton, NSlider } from 'naive-ui'
import { Icon } from '@iconify/vue'
import { usePlayerStore } from '@/stores/player'
import PlayerFullscreen from './PlayerFullscreen.vue'

const player = usePlayerStore()

const dragging = ref(false)
const dragValue = ref(0)

function fmt(sec: number) {
  if (!isFinite(sec) || sec < 0) return '00:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

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
</script>

<template>
  <div class="player-bar" v-if="player.currentTrack" @click="player.openFullPlayer()">
    <!-- 左：封面 + 信息（点击展开全屏播放器） -->
    <div class="pb-left">
      <div class="cover">
        <img v-if="player.currentTrack.cover_url" :src="player.currentTrack.cover_url" alt="" />
        <NIcon v-else :size="26" color="#999" class="cover-fallback">
          <Icon icon="material-symbols:music-note" />
        </NIcon>
      </div>
      <div class="meta">
        <div class="title" :title="player.currentTrack.title">{{ player.currentTrack.title }}</div>
        <div class="artist" :title="player.currentTrack.artist">{{ player.currentTrack.artist || '未知艺术家' }}</div>
      </div>
    </div>

    <!-- 中：进度条（只读，拖动在主播放器内完成） -->
    <div class="pb-center" @click.stop>
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

    <!-- 右：快速控制 -->
    <div class="pb-right">
      <n-button quaternary circle @click.stop="player.toggle()">
        <template #icon>
          <n-icon size="24">
            <Icon v-if="player.playing" icon="material-symbols:pause-rounded" :width="24" />
            <Icon v-else icon="material-symbols:play-arrow-rounded" :width="24" />
          </n-icon>
        </template>
      </n-button>
      <n-button quaternary circle @click.stop="player.next()">
        <template #icon>
          <n-icon size="24"><Icon icon="material-symbols:skip-next-rounded" :width="24" /></n-icon>
        </template>
      </n-button>
    </div>
  </div>

  <PlayerFullscreen />
</template>

<style scoped>
.player-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 64px;
  background: rgba(24, 24, 28, 0.96);
  backdrop-filter: blur(12px);
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  align-items: center;
  padding: 0 14px;
  gap: 12px;
  z-index: 1000;
  color: #fff;
  cursor: pointer;
}

.pb-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}

.cover {
  width: 44px;
  height: 44px;
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
  min-width: 0;
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
  min-width: 0;
  max-width: 340px;
  display: flex;
  align-items: center;
}

.progress-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.time {
  font-size: 11px;
  color: #aaa;
  font-variant-numeric: tabular-nums;
  min-width: 32px;
  text-align: center;
}

.progress-slider {
  flex: 1;
}

.pb-right {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

/* 移动端：隐藏右侧控制（点击进入全屏操作），保留封面/标题/进度 */
@media (max-width: 767px) {
  .pb-right {
    display: none;
  }

  .pb-center {
    max-width: 46%;
  }
}
</style>
