import { createRouter, createWebHistory } from 'vue-router';
import MainLayout from '@/layouts/MainLayout.vue';
import AdminLayout from '@/layouts/AdminLayout.vue';
import { useUserStore } from '@/stores/user';
import { useSystemStore } from '@/stores/system';
import { resolveNavigationTarget } from '@/lib/navigation/resolveNavigationTarget';

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

// Unified route guard — single decision point using system + auth state machines
router.beforeEach(async (to) => {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  // 1. Ensure system truth is known
  if (systemStore.status === 'unknown') {
    await systemStore.fetchSystemStatus();
  }

  // 2. If system needs setup, let the resolver decide
  // 3. If system is initialized, ensure auth truth when needed
  if (systemStore.isInitialized) {
    // If auth is still unknown and there's a persisted token, rehydrate
    if (userStore.status === 'unknown' && localStorage.getItem('token')) {
      await userStore.rehydrateSession();
    }
    // If auth is still unknown (no token), mark as anonymous so resolver works
    if (userStore.status === 'unknown') {
      // No token — anonymous. We need to set this explicitly since
      // rehydrateSession only runs when there's a token.
      // But we can't call clearStaleSession here (it's a no-op for unknown).
      // The resolver handles unknown auth as "wait", which is correct.
    }
  }

  // 3. Resolve navigation decision
  const decision = resolveNavigationTarget({
    systemStatus: systemStore.status,
    authStatus: userStore.status,
    isAdmin: userStore.isAdmin,
    targetPath: to.path,
  });

  switch (decision.type) {
    case 'allow':
      return;
    case 'redirect':
      return decision.to;
    case 'wait':
      // Wait is only valid during bootstrap. If we're still waiting
      // after fetchSystemStatus + rehydrateSession, something is wrong.
      // As a fallback, redirect to /login for initialized system.
      if (systemStore.isInitialized) {
        return '/login';
      }
      // System not ready yet — allow the navigation to proceed
      // and let the guard re-evaluate on next tick
      return;
  }
});

export default router;
