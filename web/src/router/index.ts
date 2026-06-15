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

/**
 * Centralized route revalidation.
 *
 * Re-runs resolveNavigationTarget against the current route whenever
 * system/auth/role state changes. This ensures that state transitions
 * (logout, login, setup completion, auth invalidation) immediately
 * invalidate the current page — not just future navigations.
 */
let lastRevalidationKey = '';

function revalidateCurrentRoute() {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const currentRoute = router.currentRoute.value;
  const targetPath = currentRoute.path;

  // Build a simple key to avoid redundant revalidation
  const key = `${systemStore.status}|${userStore.status}|${userStore.isAdmin}|${targetPath}`;
  if (key === lastRevalidationKey) return;
  lastRevalidationKey = key;

  const decision = resolveNavigationTarget({
    systemStatus: systemStore.status,
    authStatus: userStore.status,
    isAdmin: userStore.isAdmin,
    targetPath,
  });

  if (decision.type === 'redirect' && decision.to !== targetPath) {
    router.replace(decision.to).catch(() => {
      // ignore duplicate navigation
    });
  }
}

// Subscribe to store changes for route revalidation
let unsubscribe: (() => void) | null = null;

function setupRouteRevalidation() {
  if (unsubscribe) return; // already set up

  const systemStore = useSystemStore();
  const userStore = useUserStore();

  // Watch all three truth sources
  const stopSystem = systemStore.$subscribe(() => {
    revalidateCurrentRoute();
  });
  const stopUser = userStore.$subscribe(() => {
    revalidateCurrentRoute();
  });

  unsubscribe = () => {
    stopSystem();
    stopUser();
  };
}

// Unified route guard — single decision point using system + auth state machines
router.beforeEach(async (to) => {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  // 1. Ensure system truth is known
  if (systemStore.status === 'unknown') {
    await systemStore.fetchSystemStatus();
  }

  // 2. If system is initialized, ensure auth truth when needed
  if (systemStore.isInitialized) {
    // If auth is still unknown and there's a persisted token, rehydrate
    if (userStore.status === 'unknown' && localStorage.getItem('token')) {
      await userStore.rehydrateSession();
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
      // Set up revalidation on first successful navigation
      setupRouteRevalidation();
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
