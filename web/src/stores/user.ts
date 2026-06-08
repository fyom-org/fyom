import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login, getMe, type MeData } from '@/api/auth'

export const useUserStore = defineStore('user', () => {
  const token = ref<string>(localStorage.getItem('token') ?? '')
  const user = ref<MeData | null>(null)

  const isLoggedIn = computed(() => token.value.length > 0)

  async function doLogin(username: string, password: string): Promise<void> {
    const res = await login({ username, password })
    token.value = res.data.access_token
    localStorage.setItem('token', token.value)
  }

  async function fetchMe(): Promise<void> {
    const res = await getMe()
    user.value = res.data
  }

  function logout(): void {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  return { token, user, isLoggedIn, doLogin, fetchMe, logout }
})
