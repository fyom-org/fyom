import { nextTick } from 'vue';
import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
  type RouteLocationRaw,
} from 'vue-router';
import MainLayout from '@/layouts/MainLayout.vue';
import AdminLayout from '@/layouts/AdminLayout.vue';
import { useUserStore } from '@/stores/user';
import { useSystemStore } from '@/stores/system';
import { resolveNavigationTarget } from '@/lib/navigation/resolveNavigationTarget';

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean;
    requiresAdmin?: boolean;
    guestOnly?: boolean;
  }
}

const LOGIN_PATH = '/login';
const REGISTER_PATH = '/register';
const DEFAULT_AUTHENTICATED_PATH = '/';
const DEFAULT_ADMIN_PATH = '/admin/libraries';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: LOGIN_PATH,
      name: 'Login',
      component: () => import('@/views/LoginView.vue'),
      meta: {
        guestOnly: true,
      },
    },
    {
      path: REGISTER_PATH,
      name: 'Register',
      component: () => import('@/views/RegisterView.vue'),
      meta: {
        guestOnly: true,
      },
    },
    {
      path: '/home',
      redirect: DEFAULT_AUTHENTICATED_PATH,
    },
    {
      path: '/',
      component: MainLayout,
      meta: {
        requiresAuth: true,
      },
      children: [
        {
          path: '',
          name: 'Home',
          component: () => import('@/views/DashboardView.vue'),
        },
        {
          path: 'library',
          name: 'Library',
          component: () => import('@/views/LibraryView.vue'),
        },
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
        {
          path: 'play/:id',
          name: 'Player',
          component: () => import('@/views/PlayerView.vue'),
        },
        {
          path: 'profile',
          name: 'Profile',
          component: () => import('@/views/ProfileView.vue'),
        },
      ],
    },
    {
      path: '/admin',
      component: AdminLayout,
      meta: {
        requiresAuth: true,
        requiresAdmin: true,
      },
      children: [
        {
          path: '',
          redirect: DEFAULT_ADMIN_PATH,
        },
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
    {
      path: '/:pathMatch(.*)*',
      redirect: DEFAULT_AUTHENTICATED_PATH,
    },
  ],
});

/**
 * Shared bootstrap promise for first-route resolution.
 *
 * This prevents concurrent navigations from racing system bootstrap and session
 * rehydration, especially on hard refresh of protected deep links.
 */
let navigationBootstrapPromise: Promise<void> | null = null;

/**
 * Route revalidation subscription management.
 */
let unsubscribeRevalidation: (() => void) | null = null;
let lastRevalidationKey = '';

function isGuestPath(path: string): boolean {
  return path === LOGIN_PATH || path === REGISTER_PATH;
}

function requiresAdminRoute(to: RouteLocationNormalized): boolean {
  return to.matched.some((record) => record.meta.requiresAdmin === true);
}

function requiresProtectedRoute(to: RouteLocationNormalized): boolean {
  return to.matched.some((record) => record.meta.requiresAuth === true);
}

function readPersistedToken(): string | null {
  if (typeof window === 'undefined') return null;

  return window.localStorage.getItem('token');
}

function isSafeRedirectPath(value: string): boolean {
  if (!value.startsWith('/')) return false;
  if (value.startsWith('//')) return false;
  if (value.startsWith('/login')) return false;
  if (value.startsWith('/register')) return false;

  return true;
}

function getSafeRedirectPath(to: RouteLocationNormalized): string {
  const candidate = to.fullPath || to.path || DEFAULT_AUTHENTICATED_PATH;

  return isSafeRedirectPath(candidate) ? candidate : DEFAULT_AUTHENTICATED_PATH;
}

function makeLoginRedirect(to: RouteLocationNormalized): RouteLocationRaw {
  return {
    path: LOGIN_PATH,
    query: {
      redirect: getSafeRedirectPath(to),
    },
  };
}

function normalizeRedirectTarget(
  to: RouteLocationNormalized,
  redirectTo: string
): RouteLocationRaw {
  if (redirectTo === LOGIN_PATH) {
    return makeLoginRedirect(to);
  }

  return redirectTo;
}

/**
 * Centralized route revalidation.
 *
 * Re-runs resolveNavigationTarget against the current route whenever
 * system/auth/role state changes. This ensures that state transitions such as
 * logout or session invalidation invalidate the current page immediately.
 */
function revalidateCurrentRoute(): void {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const currentRoute = router.currentRoute.value;

  if (!currentRoute || currentRoute.path === '') return;

  const key = [
    systemStore.status,
    userStore.status,
    String(userStore.isAuthenticated),
    String(userStore.isAdmin),
    currentRoute.fullPath,
  ].join('|');

  if (key === lastRevalidationKey) return;

  lastRevalidationKey = key;

  const decision = resolveNavigationTarget({
    systemStatus: systemStore.status,
    authStatus: userStore.status,
    isAdmin: userStore.isAdmin,
    targetPath: currentRoute.path,
  });

  if (decision.type !== 'redirect') {
    return;
  }

  if (decision.to === currentRoute.path || decision.to === currentRoute.fullPath) {
    return;
  }

  nextTick(() => {
    router.replace(normalizeRedirectTarget(currentRoute, decision.to)).catch(() => {
      // Ignore duplicate navigation and cancelled navigation.
    });
  });
}

function setupRouteRevalidation(): void {
  if (unsubscribeRevalidation) return;

  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const stopSystem = systemStore.$subscribe(() => {
    revalidateCurrentRoute();
  });

  const stopUser = userStore.$subscribe(() => {
    revalidateCurrentRoute();
  });

  unsubscribeRevalidation = () => {
    stopSystem();
    stopUser();
    unsubscribeRevalidation = null;
  };
}

/**
 * Resolve system truth and auth truth before protected/admin route access.
 *
 * This function is intentionally conservative:
 * - Protected routes must not render while auth is still unknown/rehydrating.
 * - Admin routes must wait until role information is restored.
 * - Desktop bootstrap auth is attempted before falling back to anonymous.
 */
async function bootstrapNavigationState(): Promise<void> {
  const systemStore = useSystemStore();
  const userStore = useUserStore();

  if (navigationBootstrapPromise) {
    await navigationBootstrapPromise;
    return;
  }

  navigationBootstrapPromise = (async () => {
    if (systemStore.status === 'unknown') {
      await systemStore.fetchSystemStatus();
    }

    if (!systemStore.isInitialized) {
      return;
    }

    if (userStore.status === 'authenticated' && userStore.token && userStore.user) {
      return;
    }

    if (userStore.status === 'rehydrating') {
      await userStore.rehydrateSession();
      return;
    }

    const persistedToken = readPersistedToken();

    if (persistedToken) {
      await userStore.rehydrateSession();
      return;
    }

    if (userStore.status === 'unknown') {
      const bootstrapped = await userStore.bootstrapDesktopAuth();

      if (!bootstrapped) {
        userStore.setAnonymous();
      }

      return;
    }

    if (userStore.status === 'error') {
      const verified = await userStore.verifySession();

      if (!verified && !readPersistedToken()) {
        userStore.setAnonymous();
      }
    }
  })().finally(() => {
    navigationBootstrapPromise = null;
  });

  await navigationBootstrapPromise;
}

/**
 * Apply final guard constraints that must hold even if resolveNavigationTarget
 * is permissive or stale.
 */
function enforceRouteAccess(
  to: RouteLocationNormalized,
  protectedRoute: boolean,
  adminRoute: boolean
): RouteLocationRaw | true {
  const userStore = useUserStore();

  if (protectedRoute && !userStore.isAuthenticated) {
    return makeLoginRedirect(to);
  }

  if (adminRoute && !userStore.isAdmin) {
    return DEFAULT_AUTHENTICATED_PATH;
  }

  return true;
}

/**
 * Unified route guard.
 *
 * Router guard is the only global place that decides login redirection.
 * API request failures must not directly navigate the app to /login.
 */
router.beforeEach(async (to) => {
  setupRouteRevalidation();

  const systemStore = useSystemStore();
  const userStore = useUserStore();

  const guestPath = isGuestPath(to.path);
  const protectedRoute = requiresProtectedRoute(to);
  const adminRoute = requiresAdminRoute(to);

  try {
    if (guestPath || protectedRoute || adminRoute) {
      await bootstrapNavigationState();
    }
  } catch (unknownError) {
    console.error('[router] bootstrapNavigationState failed:', unknownError);

    if (protectedRoute || adminRoute) {
      return makeLoginRedirect(to);
    }

    return true;
  }

  const decision = resolveNavigationTarget({
    systemStatus: systemStore.status,
    authStatus: userStore.status,
    isAdmin: userStore.isAdmin,
    targetPath: to.path,
  });

  switch (decision.type) {
    case 'allow': {
      if (guestPath && userStore.isAuthenticated) {
        return userStore.isAdmin ? DEFAULT_ADMIN_PATH : DEFAULT_AUTHENTICATED_PATH;
      }

      return enforceRouteAccess(to, protectedRoute, adminRoute);
    }

    case 'redirect': {
      return normalizeRedirectTarget(to, decision.to);
    }

    case 'wait': {
      if (guestPath) {
        return true;
      }

      if (protectedRoute || adminRoute) {
        return makeLoginRedirect(to);
      }

      return false;
    }

    default:
      return enforceRouteAccess(to, protectedRoute, adminRoute);
  }
});

export default router;
