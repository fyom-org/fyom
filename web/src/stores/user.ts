import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import { login, getMe, type MeData } from '@/api/auth';

export const useUserStore = defineStore('user', () => {
  const router = useRouter();

  const token = ref<string>(localStorage.getItem('token') ?? '');
  const user = ref<MeData | null>(null);

  const isLoggedIn = computed(() => token.value.length > 0);
  const isAdmin = computed(() => user.value?.role === 'admin');

  async function doLogin(username: string, password: string): Promise<void> {
    const res = await login({ username, password });
    const accessToken = res.data.access_token;
    if (!accessToken) throw new Error('login response missing access_token');
    token.value = accessToken;
    localStorage.setItem('token', accessToken);
    await router.push('/');
  }

  async function fetchMe(): Promise<void> {
    const res = await getMe();
    user.value = res.data;
  }

  function clearStaleSession(): void {
    token.value = '';
    user.value = null;
    localStorage.removeItem('token');
  }

  function logout(): void {
    clearStaleSession();
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('auth:unauthorized', clearStaleSession);
  }

  return { token, user, isLoggedIn, isAdmin, doLogin, fetchMe, logout, clearStaleSession };
});
