import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '@/views/LoginView.vue'
import MainLayout from '@/layouts/MainLayout.vue'
import ImportView from '@/views/ImportView.vue'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: MainLayout,
      meta: { requiresAuth: true },
      children: [
        { path: 'import', name: 'import', component: ImportView },
        { path: 'library', name: 'Library', component: () => import('@/views/LibraryView.vue') },
        { path: 'library/:id', name: 'MediaDetail', component: () => import('@/views/MediaDetailView.vue') },
        { path: 'play/:id', name: 'Player', component: () => import('@/views/PlayerView.vue') },
        { path: 'profile', name: 'Profile', component: () => import('@/views/ProfileView.vue') },
        { path: 'home', redirect: '/import' },
        { path: '', redirect: '/import' },
      ],
    },
    { path: '/setup', name: 'Setup', component: () => import('@/views/SetupView.vue') },
    { path: '/register', name: 'Register', component: () => import('@/views/RegisterView.vue') },
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
