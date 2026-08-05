<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NDescriptions, NDescriptionsItem } from 'naive-ui'
import { getSystemInfo } from '@/api/settings'

const info = ref({ version: '', name: '' })

onMounted(async () => {
  const res = await getSystemInfo()
  info.value = res.data
})
</script>

<template>
  <div>
    <h2>系统信息</h2>
    <n-card>
      <n-descriptions bordered :column="1">
        <n-descriptions-item label="名称">{{ info.name }}</n-descriptions-item>
        <n-descriptions-item label="版本">{{ info.version }}</n-descriptions-item>
      </n-descriptions>
    </n-card>
  </div>
</template>

<style scoped>
h2 { margin-bottom: 16px; }
</style>
