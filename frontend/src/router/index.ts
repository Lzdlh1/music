import { createRouter, createWebHistory } from 'vue-router'
import Login from '../pages/Login.vue'
import Settings from '../pages/Settings.vue'
import Tasks from '../pages/Tasks.vue'

const routes = [
  { path: '/', component: Tasks },
  { path: '/login', component: Login },
  { path: '/settings', component: Settings }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
