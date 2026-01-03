<template>
  <div>
    <h2>Settings</h2>
    <label>China Mobile 云盘 Cookie (存于 localStorage)</label>
    <textarea v-model="cookies" rows="6" style="width:100%"></textarea>
    <div>
      <button @click="save">保存</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useSettingsStore } from '../stores/settings'

const store = useSettingsStore()
const cookies = ref(store.cmcookies)

function save() {
  store.setCookies(cookies.value)
  alert('已保存到 localStorage — 仅在发起任务时发送到后端')
}
</script>
