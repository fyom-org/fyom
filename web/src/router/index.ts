import { createRouter, createWebHistory } from 'vue-router';
import LoginView from '@/views/LoginView.vue';
import MainLayout from '@/layouts/MainLayout.vue';
import AdminLayout from '@/layouts/AdminLayout.vue';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginView },
    { path: '/register', name: 'Register', component: () => import('@/views/RegisterView.vue') },
    { path: '/setup', name: 'Setup', component: () => import('@/views/SetupView.vue') },
    { path: '/home', redirect: '/' },
    {
      path: '/',
      component: MainLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'Home', component: () => import('@/views/DashboardView.vue') },
        { path: 'library', name: 'Library', component: () => import('@/views/LibraryView.vue') },
        { path: 'library/:libraryId', name: 'LibraryFiltered', component: () => import('@/views/LibraryView.vue') },
        { path: 'media/:id', name: 'MediaDetail', component: () => import('@/views/MediaDetailView.vue') },
        { path: 'play/:id', name: 'Player', component: () => import('@/views/PlayerView.vue') },
        { path: 'profile', name: 'Profile', component: () => import('@/views/ProfileView.vue') },
      ],
    },
    {
      path: '/admin',
      component: AdminLayout,
      meta: { requiresAuth: true, requiresAdmin: true },
      children: [
        {
          path: 'libraries',
          name: 'AdminLibraries',
          component: () => import('@/views/admin/LibrariesView.vue'),
        },
        {
          path: 'media',
          name: 'AdminMedia',
          component: () => import('@/views/admin/MediaView.vue'),
        },
        {
          path: 'missing',
          name: 'AdminMissing',
          component: () => import('@/views/admin/MissingView.vue'),
        },
        {
          path: 'permissions',
          name: 'AdminPermissions',
          component: () => import('@/views/admin/PermissionsView.vue'),
        },
        {
          path: 'providers',
          name: 'AdminProviders',
          component: () => import('@/views/admin/ProvidersView.vue'),
        },
        {
          path: 'system',
          name: 'AdminSystem',
          component: () => import('@/views/admin/SystemView.vue'),
        },
        {
          path: 'settings',
          name: 'AdminSettings',
          component: () => import('@/views/admin/SettingsView.vue'),
        },
      ],
    },
  ],
});

// Security: role is verified server-side via /auth/me endpoint.
// localStorage is never used for role storage.
router.beforeEach(async (to) => {
  const token = localStorage.getItem('token');

  // Public routes
  if (to.path === '/login' || to.path === '/register' || to.path === '/setup') {
    return;
  }

  // Not authenticated — redirect to login
  if (!token) {
    return router.push('/login');
  }

  // Admin routes: verify role via API, not localStorage
  if (to.path.startsWith('/admin')) {
    try {
      const res = await fetch('/api/v1/auth/me', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) {
        return router.push('/login');
      }
      const data = await res.json();
      if (data.data?.role !== 'admin') {
        return router.push('/');
      }
    } catch {
      return router.push('/login');
    }
  }
});

export default router;