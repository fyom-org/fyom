import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '@/views/LoginView.vue'
import HomeView from '@/views/HomeView.vue'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginView },
    { path: '/home', name: 'home', component: HomeView, meta: { requiresAuth: true } },
    { path: '/', redirect: '/login' },
  ],
})

router.beforeEach((to) => {
  if (to.meta.requiresAuth) {
    const store = useUserStore()
    if (!store.isLoggedIn) {
      return { name: 'login' }
    }
  }
  return true
})

export default router
