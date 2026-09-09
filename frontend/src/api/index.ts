import axios from 'axios'
import NProgress from 'nprogress'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

api.interceptors.request.use((config) => {
  NProgress.start()
  const token = localStorage.getItem('mf_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => {
    NProgress.done()
    return response
  },
  (error) => {
    NProgress.done()
    if (error.response?.status === 401) {
      localStorage.removeItem('mf_token')
      localStorage.removeItem('mf_user_id')
      localStorage.removeItem('mf_username')
      localStorage.removeItem('mf_role')
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  },
)

/** 为需要 query token 鉴权的流媒体 URL 附加 token（<audio> 无法携带 Authorization 头） */
export function streamUrl(path: string) {
  const token = localStorage.getItem('mf_token')
  const sep = path.includes('?') ? '&' : '?'
  return token ? `${path}${sep}token=${encodeURIComponent(token)}` : path
}

export default api
