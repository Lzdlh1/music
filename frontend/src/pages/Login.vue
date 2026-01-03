<template>
  <div>
    <h2>Login</h2>
    <form @submit.prevent="doLogin">
      <input v-model="username" placeholder="username" />
      <input v-model="password" type="password" placeholder="password" />
      <button type="submit">Login</button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRouter } from 'vue-router'

const username = ref('')
const password = ref('')
const auth = useAuthStore()
const router = useRouter()

async function doLogin() {
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } catch (e) {
    alert('login failed')
  }
}
</script>
