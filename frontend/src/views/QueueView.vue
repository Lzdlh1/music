<script setup lang="ts">
import { onMounted } from 'vue'
import { NButton, NSpace, NCollapse, NCollapseItem, NEmpty, useMessage } from 'naive-ui'
import { useTaskStore } from '@/stores/task'
import { pauseTask, cancelTask } from '@/api/task'
import TaskItem from '@/components/task/TaskItem.vue'

const taskStore = useTaskStore()
const message = useMessage()

onMounted(() => {
  taskStore.fetchTasks()
  taskStore.fetchStats()
})

const activeStatuses = ['DOWNLOADING', 'PROCESSING', 'UPLOADING', 'FETCHING_META']

function activeTasks() {
  return taskStore.tasks.filter((t) => activeStatuses.includes(t.status))
}

function pendingTasks() {
  return taskStore.tasks.filter((t) => t.status === 'PENDING')
}

function doneTasks() {
  return taskStore.tasks.filter((t) => t.status === 'DONE')
}

function failedTasks() {
  return taskStore.tasks.filter((t) => t.status === 'FAILED')
}

async function handlePauseAll() {
  const active = activeTasks()
  if (!active.length) { message.info('没有进行中的任务'); return }
  for (const t of active) {
    try { await pauseTask(t.id) } catch { /* skip */ }
  }
  message.success(`已暂停 ${active.length} 个任务`)
  taskStore.fetchTasks()
}

async function handleClearDone() {
  const done = doneTasks()
  if (!done.length) { message.info('没有已完成的任务'); return }
  for (const t of done) {
    try { await cancelTask(t.id) } catch { /* skip */ }
  }
  message.success(`已清除 ${done.length} 个已完成任务`)
  taskStore.fetchTasks()
}
</script>

<template>
  <div class="queue-page">
    <div class="queue-header">
      <h1 class="page-title">下载队列</h1>
      <n-space>
        <n-button size="small" @click="handlePauseAll">全部暂停</n-button>
        <n-button size="small" @click="handleClearDone">清空已完成</n-button>
      </n-space>
    </div>

    <div v-if="activeTasks().length" class="section">
      <h3>进行中 ({{ activeTasks().length }})</h3>
      <TaskItem v-for="task in activeTasks()" :key="task.id" :task="task" />
    </div>

    <n-collapse>
      <n-collapse-item :title="`等待中 (${pendingTasks().length})`" v-if="pendingTasks().length">
        <TaskItem v-for="task in pendingTasks()" :key="task.id" :task="task" />
      </n-collapse-item>
      <n-collapse-item :title="`已完成 (${doneTasks().length})`" v-if="doneTasks().length">
        <TaskItem v-for="task in doneTasks()" :key="task.id" :task="task" />
      </n-collapse-item>
      <n-collapse-item :title="`失败 (${failedTasks().length})`" v-if="failedTasks().length">
        <TaskItem v-for="task in failedTasks()" :key="task.id" :task="task" />
      </n-collapse-item>
    </n-collapse>

    <n-empty v-if="!taskStore.tasks.length" description="暂无下载任务" />
  </div>
</template>

<style scoped>
.queue-page {
  max-width: 900px;
  margin: 0 auto;
}

.queue-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-title {
  font-size: 24px;
  font-weight: 700;
}

.section {
  margin-bottom: 16px;
}

.section h3 {
  font-size: 14px;
  color: #666;
  margin-bottom: 8px;
}
</style>
