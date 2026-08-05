<script setup lang="ts">
import { NCard, NProgress, NButton, NSpace, NTag } from 'naive-ui'
import type { Task } from '@/types'
import { pauseTask, resumeTask, cancelTask } from '@/api/task'

const props = defineProps<{
  task: Task
}>()

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    PENDING: '等待中',
    FETCHING_META: '获取信息',
    DOWNLOADING: '下载中',
    PROCESSING: '处理中',
    UPLOADING: '上传中',
    DONE: '已完成',
    FAILED: '失败',
    PAUSED: '已暂停',
    CANCELLED: '已取消',
  }
  return labels[status] || status
}

function statusType(status: string) {
  if (['DONE'].includes(status)) return 'success'
  if (['FAILED'].includes(status)) return 'error'
  if (['DOWNLOADING', 'UPLOADING', 'PROCESSING'].includes(status)) return 'info'
  return 'default'
}

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec <= 0) return ''
  const mb = bytesPerSec / (1024 * 1024)
  if (mb >= 1) return `${mb.toFixed(1)}MB/s`
  return `${(bytesPerSec / 1024).toFixed(0)}KB/s`
}

async function handlePause() {
  await pauseTask(props.task.id)
}

async function handleResume() {
  await resumeTask(props.task.id)
}

async function handleCancel() {
  await cancelTask(props.task.id)
}
</script>

<template>
  <n-card size="small" class="task-item">
    <div class="task-row">
      <div class="task-info">
        <div class="task-title">
          {{ task.track_info?.title || '未知曲目' }}
          <span class="task-artist">- {{ task.track_info?.artist || '未知' }}</span>
        </div>
        <div class="task-status-row">
          <n-tag size="tiny" :type="statusType(task.status)" round>
            {{ statusLabel(task.status) }}
          </n-tag>
          <span v-if="task.progress && task.progress.percent > 0" class="task-progress-text">
            {{ task.progress.percent.toFixed(1) }}%
          </span>
          <span v-if="task.progress?.speed" class="task-speed">
            {{ formatSpeed(task.progress.speed) }}
          </span>
          <span v-if="task.progress?.eta" class="task-eta">
            ETA: {{ task.progress.eta }}s
          </span>
        </div>
        <n-progress
          v-if="task.progress && task.progress.percent > 0"
          type="line"
          :percentage="task.progress.percent"
          :show-indicator="false"
          :height="4"
          style="margin-top: 4px"
        />
        <div v-if="task.error" class="task-error">{{ task.error }}</div>
      </div>
      <n-space class="task-actions" size="small">
        <n-button
          v-if="['DOWNLOADING', 'UPLOADING'].includes(task.status)"
          size="tiny"
          @click="handlePause"
        >
          暂停
        </n-button>
        <n-button
          v-if="task.status === 'PAUSED'"
          size="tiny"
          type="primary"
          @click="handleResume"
        >
          恢复
        </n-button>
        <n-button
          v-if="!['DONE', 'CANCELLED'].includes(task.status)"
          size="tiny"
          type="error"
          @click="handleCancel"
        >
          取消
        </n-button>
      </n-space>
    </div>
  </n-card>
</template>

<style scoped>
.task-item {
  border-radius: 12px;
  margin-bottom: 8px;
}

.task-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.task-info {
  flex: 1;
  min-width: 0;
}

.task-title {
  font-weight: 600;
  font-size: 14px;
}

.task-artist {
  font-weight: 400;
  color: #999;
}

.task-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  font-size: 12px;
  color: #999;
}

.task-error {
  font-size: 12px;
  color: #f43f5e;
  margin-top: 4px;
}

.task-actions {
  flex-shrink: 0;
}
</style>
