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
 * | ---------------- | ------------------- | ------------------ | ------------------- |
 * | unknown/checking | *                   | *                   | wait                |
 * | needs_setup      | *                   | /setup              | allow               |
 * | needs_setup      | *                   | != /setup           | redirect /setup     |
 * | initialized      | unknown/rehydrating | *                   | wait                |
 * | initialized      | anonymous           | /login, /register   | allow               |
 * | initialized      | anonymous           | protected/admin     | redirect /login     |
 * | initialized      | authenticated       | /setup              | redirect /          |
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

  // 2. System needs setup — only /setup is allowed
  if (systemStatus === 'needs_setup') {
    if (targetPath === '/setup') {
      return { type: 'allow' };
    }
    return { type: 'redirect', to: '/setup' };
  }

  // 3. System is initialized
  if (systemStatus === 'initialized') {
    // Auth not yet determined — wait
    if (authStatus === 'unknown' || authStatus === 'rehydrating') {
      return { type: 'wait' };
    }

    // Authenticated users
    if (authStatus === 'authenticated') {
      // /setup is no longer valid after initialization
      if (targetPath === '/setup') {
        return { type: 'redirect', to: '/' };
      }
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
      // /setup should not be reachable after initialization
      if (targetPath === '/setup') {
        return { type: 'redirect', to: '/login' };
      }
      // Everything else requires auth
      return { type: 'redirect', to: '/login' };
    }
  }

  // Fallback: wait
  return { type: 'wait' };
}
