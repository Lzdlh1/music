<script setup lang="ts">
import { NCard, NInput, NSpace, NRadioGroup, NRadio, NButton, useMessage } from 'naive-ui'
import { ref, computed, onMounted, watch } from 'vue'
import { getSetting, updateSetting } from '@/api/settings'

const message = useMessage()

const presets = [
  {
    label: '按歌手/专辑分类',
    value: '{artist}/{album}/{track_no:02d} - {title}.{ext}',
    example: '周杰伦/范特西/05 - 爱在西元前.flac',
  },
  {
    label: '按歌手/专辑(年份)分类',
    value: '{artist}/{album} ({year})/{track_no:02d} - {title}.{ext}',
    example: '周杰伦/范特西 (2001)/05 - 爱在西元前.flac',
  },
  {
    label: '按歌手分类（不分专辑）',
    value: '{artist}/{title}.{ext}',
    example: '周杰伦/爱在西元前.flac',
  },
  {
    label: '歌名同名文件夹（歌曲/歌词/封面放一起）',
    value: '{title}/{title}.{ext}',
    example: '爱在西元前/爱在西元前.flac',
  },
  {
    label: '全部放在一起',
    value: '{artist} - {title}.{ext}',
    example: '周杰伦 - 爱在西元前.flac',
  },
  {
    label: '自定义',
    value: '__custom__',
    example: '',
  },
]

const selectedPreset = ref(presets[0].value)
const customTemplate = ref('{artist}/{album}/{track_no:02d} - {title}.{ext}')

const currentTemplate = computed(() => {
  if (selectedPreset.value === '__custom__') return customTemplate.value
  return selectedPreset.value
})

const preview = computed(() => {
  const info: Record<string, string> = {
    '{artist}': '周杰伦',
    '{album_artist}': '周杰伦',
    '{album}': '范特西',
    '{title}': '爱在西元前',
    '{year}': '2001',
    '{track_no:02d}': '05',
    '{track_no}': '5',
    '{disc_no}': '1',
    '{genre}': 'Pop',
    '{ext}': 'flac',
    '{quality}': 'FLAC',
    '{source}': 'netease',
  }
  let result = currentTemplate.value
  for (const [k, v] of Object.entries(info)) {
    result = result.replaceAll(k, v)
  }
  return result
})

onMounted(async () => {
  try {
    const res = await getSetting('naming')
    const data = res.data?.data
    if (data?.template) {
      const match = presets.find((p) => p.value === data.template)
      if (match) {
        selectedPreset.value = match.value
      } else {
        selectedPreset.value = '__custom__'
        customTemplate.value = data.template
      }
    }
  } catch { /* use default */ }
})

async function handleSave() {
  await updateSetting('naming', { template: currentTemplate.value })
  message.success('命名规则已保存')
}
</script>

<template>
  <div>
    <h2>文件保存方式</h2>
    <n-card>
      <n-space vertical :size="16">
        <n-radio-group v-model:value="selectedPreset">
          <n-space vertical :size="12">
            <n-radio
              v-for="preset in presets"
              :key="preset.value"
              :value="preset.value"
            >
              <span>{{ preset.label }}</span>
              <span v-if="preset.example" style="margin-left: 12px; color: #999; font-size: 13px;">
                {{ preset.example }}
              </span>
            </n-radio>
          </n-space>
        </n-radio-group>

        <div v-if="selectedPreset === '__custom__'" style="margin-top: 8px;">
          <n-input
            v-model:value="customTemplate"
            placeholder="输入自定义模板"
          />
          <p style="margin-top: 8px; color: #999; font-size: 12px;">
            可用变量：{artist}、{album_artist}、{album}、{title}、{year}、
            {track_no}、{track_no:02d}、{disc_no}、{genre}、{ext}、{quality}、{source}
          </p>
        </div>

        <div style="padding: 12px; background: #f9f9f9; border-radius: 6px; font-size: 13px;">
          <span style="color: #666;">保存路径预览：</span>
          <code style="color: #6366f1;">{{ preview }}</code>
        </div>

        <n-button type="primary" @click="handleSave">保存</n-button>
      </n-space>
    </n-card>
  </div>
</template>

<style scoped>
h2 { margin-bottom: 16px; }
</style>
