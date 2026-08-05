import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useWebSocket } from '@vueuse/core'
import type { WSMessage, TaskUpdateData } from '@/types'
import { useTaskStore } from './task'

export const useWSStore = defineStore('ws', () => {
  const connected = ref(false)
  const wsUrl = ref('')

  function connect() {
    const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
    wsUrl.value = `${protocol}://${location.host}/ws/tasks`

    const { status, data } = useWebSocket(wsUrl.value, {
      autoReconnect: {
        retries: -1,
        delay: 3000,
      },
      heartbeat: {
        message: 'ping',
        interval: 30000,
      },
    })

    // 监听连接状态
    const checkStatus = () => {
      connected.value = status.value === 'OPEN'
    }

    // 监听 data 变化
    const processMessage = () => {
      if (!data.value) return
      try {
        const msg: WSMessage = JSON.parse(data.value)
        handleMessage(msg)
      } catch {
        // 忽略无效消息
      }
    }

    // 使用 setInterval 简单轮询状态（因为 useWebSocket 返回的是 ref）
    setInterval(checkStatus, 1000)
    setInterval(processMessage, 100)

    return { status, data }
  }

  function handleMessage(msg: WSMessage) {
    const taskStore = useTaskStore()

    switch (msg.type) {
      case 'task_update':
        taskStore.handleTaskUpdate(msg.data as TaskUpdateData)
        break
      case 'task_done':
      case 'task_failed':
        taskStore.fetchStats()
        break
      case 'queue_stats':
        taskStore.fetchStats()
        break
    }
  }

  return { connected, connect }
})
