import { nextTick } from 'vue';
import { createRouter, createWebHistory, type RouteLocationNormalized } from 'vue-router';
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
 * Shared bootstrap promise for first-route resolution.
 *
 * This prevents concurrent navigations from racing system bootstrap and
 * session rehydration, especially on hard refresh of protected deep links
 * such as /admin/libraries.
 */
let navigationBootstrapPromise: Promise<void> | null = null;

/**
 * Route revalidation subscription management.
 */
let unsubscribe: (() => void) | null = null;
let lastRevalidationKey = '';

function isGuestPath(path: string): boolean {
  return path === '/login' || path === '/register';
}

function requiresAdminRoute(to: RouteLocationNormalized): boolean {
  return to.matched.some((record) => record.meta.requiresAdmin === true);
}

function requiresProtectedRoute(to: RouteLocationNormalized): boolean {
  return to.matched.some((record) => record.meta.requiresAuth === true);
}

/**
 * Centralized route revalidation.
 *
 * Re-runs resolveNavigationTarget against the current route whenever
 * system/auth/role state changes. This ensures that state transitions
 * (logout, login, auth invalidation) immediately invalidate the current
 * page — not just future navigations.
 *
 * Uses nextTick() to defer navigation, preventing conflicts with
 * Vue Router's internal navigation state when the subscriber fires
 * synchronously during an async operation (e.g. rehydrateSession).
 */
function revalidateCurrentRoute() {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const currentRoute = router.currentRoute.value;
  const targetPath = currentRoute.path;

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
    nextTick(() => {
      router.replace(decision.to).catch(() => {
        // ignore duplicate navigation
      });
    });
  }
}

function setupRouteRevalidation() {
  if (unsubscribe) return;

  const systemStore = useSystemStore();
  const userStore = useUserStore();

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

/**
 * Ensure system truth and auth truth are settled before we let protected
 * or admin routes proceed. This is the critical fix for hard-refresh on
 * protected deep links, especially /admin/*.
 */
async function bootstrapNavigationState(to: RouteLocationNormalized): Promise<void> {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  if (navigationBootstrapPromise) {
    await navigationBootstrapPromise;
    return;
  }

  navigationBootstrapPromise = (async () => {
    // 1. Resolve system status first
    if (systemStore.status === 'unknown') {
      await systemStore.fetchSystemStatus();
    }

    // 2. If system is initialized, auth truth must be resolved before
    // protected/admin routes are allowed to proceed.
    if (systemStore.isInitialized) {
      const persistedToken = localStorage.getItem('token');

      if (userStore.status === 'unknown') {
        if (persistedToken) {
          await userStore.rehydrateSession();
        } else {
          userStore.setAnonymous();
        }
      } else if (userStore.status === 'rehydrating') {
        // Never allow protected/admin deep links to proceed while role/auth
        // restoration is still in flight.
        await userStore.rehydrateSession();
      }
    }

    // 3. Guest routes may still be visited anonymously after bootstrap.
    // No extra action required here.
    void to;
  })().finally(() => {
    navigationBootstrapPromise = null;
  });

  await navigationBootstrapPromise;
}

/**
 * Unified route guard — single decision point using system + auth state machines.
 *
 * Critical behavior:
 * - We do NOT allow protected/admin routes to slip through while auth is still
 *   "rehydrating".
 * - Admin deep links must wait until user + role have been restored.
 */
router.beforeEach(async (to) => {
  setupRouteRevalidation();

  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const guestPath = isGuestPath(to.path);
  const protectedRoute = requiresProtectedRoute(to);
  const adminRoute = requiresAdminRoute(to);

  try {
    // Bootstrap is required for:
    // 1. guest routes (to correctly redirect authenticated users away from /login)
    // 2. protected routes
    // 3. admin routes
    if (guestPath || protectedRoute || adminRoute) {
      await bootstrapNavigationState(to);
    }
  } catch (err) {
    console.error('[router] bootstrapNavigationState failed:', err);

    // Fail closed for protected/admin routes.
    if (protectedRoute || adminRoute) {
      return '/login';
    }

    // Guest routes may still proceed.
    return true;
  }

  const decision = resolveNavigationTarget({
    systemStatus: systemStore.status,
    authStatus: userStore.status,
    isAdmin: userStore.isAdmin,
    targetPath: to.path,
  });

  switch (decision.type) {
    case 'allow':
      return true;

    case 'redirect':
      return decision.to;

    case 'wait':
      // Guest routes can still render while anonymous/system settles.
      if (guestPath) {
        return true;
      }

      // Important: protected/admin routes must NOT be allowed through
      // in wait state after bootstrap, otherwise deep-link refresh can
      // render into a half-restored auth/role state and black-screen.
      if (protectedRoute || adminRoute) {
        return false;
      }

      return false;
  }
});

export default router;
