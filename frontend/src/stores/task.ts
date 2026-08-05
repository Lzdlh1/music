import { defineStore } from 'pinia'
import { ref } from 'vue'
import { listTasks, getTaskStats } from '@/api/task'
import type { Task, TaskStats, TaskUpdateData } from '@/types'

export const useTaskStore = defineStore('task', () => {
  const tasks = ref<Task[]>([])
  const stats = ref<TaskStats>({
    PENDING: 0,
    DOWNLOADING: 0,
    PROCESSING: 0,
    UPLOADING: 0,
    DONE: 0,
    FAILED: 0,
    PAUSED: 0,
  })
  const loading = ref(false)

  async function fetchTasks(status?: string, page = 1) {
    loading.value = true
    try {
      const res = await listTasks(status, page)
      tasks.value = res.data.data || []
    } finally {
      loading.value = false
    }
  }

  async function fetchStats() {
    const res = await getTaskStats()
    stats.value = res.data.data as TaskStats
  }

  function handleTaskUpdate(data: TaskUpdateData) {
    const idx = tasks.value.findIndex((t) => t.id === data.task_id)
    if (idx >= 0) {
      tasks.value[idx].status = data.status
      tasks.value[idx].progress = data.progress
    }
  }

  const activeCount = () =>
    stats.value.DOWNLOADING + stats.value.PROCESSING + stats.value.UPLOADING + stats.value.PENDING

  return { tasks, stats, loading, fetchTasks, fetchStats, handleTaskUpdate, activeCount }
})
