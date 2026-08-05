<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NSwitch, NSelect, NSpace, NButton, useMessage } from 'naive-ui'
import { getSetting, updateSetting } from '@/api/settings'

const message = useMessage()

const qualityOptions = [
  { label: 'FLAC (无损)', value: 'FLAC' },
  { label: '320K', value: '320K' },
  { label: '128K', value: '128K' },
]

const form = ref({
  default_quality: 'FLAC',
  embed_cover: true,
  embed_lyrics: true,
  save_lrc_file: true,
})

onMounted(async () => {
  try {
    const res = await getSetting('download')
    if (res.data?.data) Object.assign(form.value, res.data.data)
  } catch { /* use defaults */ }
})

async function handleSave() {
  await updateSetting('download', form.value)
  message.success('下载偏好已保存')
}
</script>

<template>
  <div>
    <h2>下载偏好</h2>
    <n-card>
      <n-space vertical>
        <div class="setting-row">
          <span>默认音质</span>
          <n-select :options="qualityOptions" v-model:value="form.default_quality" style="width: 200px" />
        </div>
        <div class="setting-row">
          <span>内嵌封面到音频</span>
          <n-switch v-model:value="form.embed_cover" />
        </div>
        <div class="setting-row">
          <span>内嵌歌词到音频</span>
          <n-switch v-model:value="form.embed_lyrics" />
        </div>
        <div class="setting-row">
          <span>保存独立 .lrc 文件</span>
          <n-switch v-model:value="form.save_lrc_file" />
        </div>
        <n-button type="primary" @click="handleSave" style="margin-top: 8px;">保存</n-button>
      </n-space>
    </n-card>
  </div>
</template>

<style scoped>
h2 { margin-bottom: 16px; }
.setting-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
}
</style>
