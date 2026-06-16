export type RouteDecision =
  | { type: 'allow' }
  | { type: 'redirect'; to: string }
  | { type: 'wait' };

export interface ResolveNavigationInput {
  systemStatus: string;
  authStatus: string;
  isAdmin: boolean;
  targetPath: string;
}

/**
 * Pure route-policy resolver.
 *
 * Single decision table for all navigation in fyom.
 *
 * | System           | Auth                | Target              | Decision            |
 * | ---------------- | ------------------- | ------------------- | ------------------- |
 * | unknown/checking | *                   | *                   | wait                |
 * | initialized      | unknown/rehydrating | *                   | wait                |
 * | initialized      | anonymous           | /login, /register   | allow               |
 * | initialized      | anonymous           | protected/admin     | redirect /login     |
 * | initialized      | authenticated       | /login, /register   | redirect /          |
 * | initialized      | authenticated       | protected          | allow               |
 * | initialized      | authenticated       | admin + isAdmin     | allow               |
 * | initialized      | authenticated       | admin + !isAdmin    | redirect /          |
 * | error            | *                   | *                   | wait                |
 */
export function resolveNavigationTarget(
  input: ResolveNavigationInput,
): RouteDecision {
  const { systemStatus, authStatus, isAdmin, targetPath } = input;

  // 1. System truth not yet known — wait
  if (systemStatus === 'unknown' || systemStatus === 'checking' || systemStatus === 'error') {
    return { type: 'wait' };
  }

  // 2. System is initialized (backend guarantees bootstrap before frontend needs routes)
  if (systemStatus === 'initialized') {
    // /setup no longer exists — redirect to login
    if (targetPath === '/setup') {
      return { type: 'redirect', to: '/login' };
    }

    // Auth not yet determined — wait
    if (authStatus === 'unknown' || authStatus === 'rehydrating') {
      return { type: 'wait' };
    }

    // Authenticated users
    if (authStatus === 'authenticated') {
      // /login, /register — redirect away if already logged in
      if (targetPath === '/login' || targetPath === '/register') {
        return { type: 'redirect', to: '/' };
      }
      // Admin routes
      if (targetPath.startsWith('/admin')) {
        if (isAdmin) {
          return { type: 'allow' };
        }
        return { type: 'redirect', to: '/' };
      }
      // All other protected routes (/, /library, /profile, /media/*, /play/*)
      return { type: 'allow' };
    }

    // Anonymous users
    if (authStatus === 'anonymous') {
      // /login and /register are the correct landing
      if (targetPath === '/login' || targetPath === '/register') {
        return { type: 'allow' };
      }
      // Everything else requires auth
      return { type: 'redirect', to: '/login' };
    }
  }

  // Fallback: wait
  return { type: 'wait' };
}
