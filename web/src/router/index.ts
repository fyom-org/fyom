import { createRouter, createWebHistory } from 'vue-router';
import MainLayout from '@/layouts/MainLayout.vue';
import AdminLayout from '@/layouts/AdminLayout.vue';
import { useUserStore } from '@/stores/user';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'Login', component: () => import('@/views/LoginView.vue') },
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
        {
          path: 'library/:libraryId',
          name: 'LibraryFiltered',
          component: () => import('@/views/LibraryView.vue'),
        },
        {
          path: 'media/:id',
          name: 'MediaDetail',
          component: () => import('@/views/MediaDetailView.vue'),
        },
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

// Public routes — always accessible
const PUBLIC_PATHS = new Set(['/login', '/register', '/setup']);

router.beforeEach(async (to) => {
  const store = useUserStore();

  // Public routes — pass through
  if (PUBLIC_PATHS.has(to.path)) {
    return;
  }

  // Not yet bootstrapped — trigger rehydration and wait.
  if (!store.isAuthReady) {
    await store.rehydrateSession();
  }

  // After rehydration, if still not authenticated — redirect.
  if (!store.isAuthenticated) {
    // No users yet? → setup flow
    return '/setup';
  }

  // Admin routes — check role from server-verified user, not localStorage token.
  if (to.meta.requiresAdmin && !store.isAdmin) {
    return '/';
  }
});

export default router;
