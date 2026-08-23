import { defineStore } from 'pinia'
import type { EqPreset, LoopMode, LyricsLine, PlayTrack } from '@/types'
import { getLibraryLyrics, parseLrc } from '@/api/library'

/** 10 段 EQ 频点（Hz） */
export const EQ_BANDS = [60, 150, 400, 1000, 2400, 4000, 6000, 8000, 10000, 12000]

/** EQ 预设 */
export const EQ_PRESETS: EqPreset[] = [
  { name: '默认', gains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] },
  { name: '流行', gains: [-1, 2, 4, 2, 0, 0, 1, 2, 3, 2] },
  { name: '摇滚', gains: [4, 3, -1, -2, -1, 2, 3, 4, 4, 3] },
  { name: '爵士', gains: [3, 1, -1, 0, -1, 0, 1, 2, 2, 1] },
  { name: '古典', gains: [3, 2, 0, -1, -1, -1, 0, 1, 2, 3] },
  { name: '低音增强', gains: [6, 5, 3, 1, 0, 0, 0, 0, 0, 0] },
  { name: '高音增强', gains: [0, 0, 0, 0, 0, 1, 2, 3, 4, 5] },
]

// ---------- 模块级音频实例（非响应式） ----------
let audioEl: HTMLAudioElement | null = null
let audioCtx: AudioContext | null = null
let filters: BiquadFilterNode[] = []
let dryGain: GainNode | null = null
let initialized = false

interface State {
  queue: PlayTrack[]
  currentIndex: number
  playing: boolean
  currentTime: number
  duration: number
  volume: number
  muted: boolean
  playbackRate: number
  loopMode: LoopMode
  shuffle: boolean
  eqEnabled: boolean
  eqGains: number[]
  eqPreset: string
  lyrics: LyricsLine[]
  lyricsLoading: boolean
  showLyrics: boolean
  showSettings: boolean
  showFullPlayer: boolean
  error: string
}

function applyEq(eqEnabled: boolean, eqGains: number[]) {
  if (!filters.length || !dryGain) return
  const maxGain = 12
  eqGains.forEach((g, i) => {
    if (filters[i]) filters[i].gain.value = Math.max(-maxGain, Math.min(maxGain, g))
  })
  dryGain.gain.value = eqEnabled ? 0 : 1
}

function initAudio(store: ReturnType<typeof usePlayerStore>) {
  if (initialized) return
  initialized = true

  const audio = new Audio()
  audioEl = audio
  audio.preload = 'metadata'

  audio.addEventListener('timeupdate', () => {
    store.currentTime = audio.currentTime
  })
  audio.addEventListener('durationchange', () => {
    store.duration = audio.duration || 0
  })
  audio.addEventListener('play', () => {
    store.playing = true
  })
  audio.addEventListener('pause', () => {
    store.playing = false
  })
  audio.addEventListener('ended', () => {
    if (store.loopMode === 'one') {
      audio.currentTime = 0
      audio.play()
    } else {
      store.next()
    }
  })
  audio.addEventListener('error', () => {
    store.error = '播放失败，请检查文件是否可访问'
    store.playing = false
  })
  audio.volume = store.volume
  audio.playbackRate = store.playbackRate

  // Web Audio：EQ 音效链
  const Ctor =
    window.AudioContext ||
    (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
  const ctx = new Ctor()
  audioCtx = ctx
  const source = ctx.createMediaElementSource(audio)
  for (const freq of EQ_BANDS) {
    const f = ctx.createBiquadFilter()
    f.type = 'peaking'
    f.frequency.value = freq
    f.Q.value = 1
    f.gain.value = 0
    filters.push(f)
  }
  dryGain = ctx.createGain()
  dryGain.gain.value = 1

  // source -> filters[0] -> ... -> filters[9] -> destination（EQ 开启时生效）
  // source -> dryGain -> destination（EQ 关闭时走直通）
  source.connect(filters[0])
  for (let i = 0; i < filters.length - 1; i++) filters[i].connect(filters[i + 1])
  filters[filters.length - 1].connect(ctx.destination)
  source.connect(dryGain)
  dryGain.connect(ctx.destination)

  applyEq(store.eqEnabled, store.eqGains)
}

function resumeCtx() {
  if (audioCtx && audioCtx.state === 'suspended') audioCtx.resume()
}

export const usePlayerStore = defineStore('player', {
  state: (): State => ({
    queue: [],
    currentIndex: -1,
    playing: false,
    currentTime: 0,
    duration: 0,
    volume: 0.8,
    muted: false,
    playbackRate: 1,
    loopMode: 'off',
    shuffle: false,
    eqEnabled: false,
    eqGains: [...EQ_PRESETS[0].gains],
    eqPreset: '默认',
    lyrics: [],
    lyricsLoading: false,
    showLyrics: false,
    showSettings: false,
    showFullPlayer: false,
    error: '',
  }),

  getters: {
    currentTrack(state): PlayTrack | null {
      return state.currentIndex >= 0 && state.currentIndex < state.queue.length
        ? state.queue[state.currentIndex]
        : null
    },
    progress(state): number {
      if (!state.duration) return 0
      return Math.min(100, (state.currentTime / state.duration) * 100)
    },
    activeLyricsIndex(state): number {
      const t = state.currentTime * 1000
      let idx = -1
      for (let i = 0; i < state.lyrics.length; i++) {
        if (state.lyrics[i].time <= t) idx = i
        else break
      }
      return idx
    },
    volumePercent(state): number {
      return Math.round(state.volume * 100)
    },
  },

  actions: {
    init() {
      initAudio(this)
    },

    // ---------- 播放控制 ----------
    async playTrack(track: PlayTrack, queue?: PlayTrack[]) {
      this.init()
      if (!audioEl) return

      if (queue && queue.length) {
        this.queue = queue
        const idx = queue.findIndex((t) => t.id === track.id)
        this.currentIndex = idx >= 0 ? idx : 0
      } else {
        const idx = this.queue.findIndex((t) => t.id === track.id)
        if (idx >= 0) {
          this.currentIndex = idx
        } else {
          this.queue = [...this.queue, track]
          this.currentIndex = this.queue.length - 1
        }
      }

      const current = this.currentTrack
      if (!current) return

      if (current.src === audioEl.src && audioEl.readyState > 0) {
        resumeCtx()
        await audioEl.play()
        return
      }

      this.error = ''
      this.lyrics = []
      audioEl.pause()
      audioEl.removeAttribute('src')
      audioEl.load()
      audioEl.src = current.src
      audioEl.playbackRate = this.playbackRate
      audioEl.volume = this.muted ? 0 : this.volume

      resumeCtx()
      try {
        await audioEl.play()
      } catch (e) {
        this.error = '浏览器阻止了自动播放，请点击播放按钮'
      }

      this.loadLyricsFor(current)
    },

    async toggle() {
      this.init()
      if (!audioEl || !this.currentTrack) return
      if (audioEl.paused) {
        resumeCtx()
        await audioEl.play()
      } else {
        audioEl.pause()
      }
    },

    async next() {
      const n = this.queue.length
      if (!n) return
      let idx: number
      if (this.shuffle && n > 1) {
        do {
          idx = Math.floor(Math.random() * n)
        } while (idx === this.currentIndex)
      } else {
        idx = this.currentIndex + 1
        if (idx >= n) {
          if (this.loopMode === 'all') idx = 0
          else {
            this.pause()
            return
          }
        }
      }
      this.currentIndex = idx
      await this.reloadCurrent()
    },

    async prev() {
      if (this.currentTime > 3 && audioEl) {
        this.seek(0)
        return
      }
      const n = this.queue.length
      if (!n) return
      let idx = this.currentIndex - 1
      if (idx < 0) idx = n - 1
      this.currentIndex = idx
      await this.reloadCurrent()
    },

    async reloadCurrent() {
      if (!audioEl) return
      const t = this.currentTrack
      if (!t) return
      this.error = ''
      this.lyrics = []
      audioEl.src = t.src
      audioEl.playbackRate = this.playbackRate
      resumeCtx()
      try {
        await audioEl.play()
      } catch {
        /* noop */
      }
      this.loadLyricsFor(t)
    },

    pause() {
      if (audioEl) audioEl.pause()
    },

    seek(time: number) {
      if (!audioEl) return
      audioEl.currentTime = time
      this.currentTime = time
    },

    seekByPercent(p: number) {
      if (!audioEl || !this.duration) return
      audioEl.currentTime = (p / 100) * this.duration
    },

    // ---------- 设置 ----------
    setVolume(v: number) {
      this.volume = Math.max(0, Math.min(1, v))
      if (audioEl) audioEl.volume = this.muted ? 0 : this.volume
    },

    toggleMute() {
      this.muted = !this.muted
      if (audioEl) audioEl.volume = this.muted ? 0 : this.volume
    },

    setPlaybackRate(rate: number) {
      this.playbackRate = rate
      if (audioEl) audioEl.playbackRate = rate
    },

    setLoopMode(mode: LoopMode) {
      this.loopMode = mode
      if (audioEl) audioEl.loop = mode === 'one'
    },

    toggleShuffle() {
      this.shuffle = !this.shuffle
    },

    setEqEnabled(enabled: boolean) {
      this.eqEnabled = enabled
      applyEq(enabled, this.eqGains)
    },

    setEqGains(gains: number[]) {
      this.eqGains = [...gains]
      applyEq(this.eqEnabled, this.eqGains)
    },

    setEqPreset(name: string) {
      const preset = EQ_PRESETS.find((p) => p.name === name)
      if (!preset) return
      this.eqPreset = name
      this.eqGains = [...preset.gains]
      applyEq(this.eqEnabled, this.eqGains)
    },

    // ---------- 歌词 ----------
    async loadLyricsFor(track: PlayTrack) {
      this.lyrics = []
      this.lyricsLoading = true
      try {
        let lrc = track.lrc
        if (!lrc && !track.from_cloud) {
          const res = await getLibraryLyrics(track.id)
          lrc = res.data?.data?.lrc || ''
        }
        this.lyrics = parseLrc(lrc || '')
      } catch {
        this.lyrics = []
      } finally {
        this.lyricsLoading = false
      }
    },

    toggleLyrics() {
      this.showLyrics = !this.showLyrics
    },

    toggleSettings() {
      this.showSettings = !this.showSettings
    },

    openFullPlayer() {
      this.showFullPlayer = true
    },

    closeFullPlayer() {
      this.showFullPlayer = false
    },

    clearQueue() {
      this.queue = []
      this.currentIndex = -1
      this.pause()
      if (audioEl) {
        audioEl.removeAttribute('src')
        audioEl.load()
      }
    },
  },
})
