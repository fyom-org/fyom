export type RouteDecision = { type: 'allow' } | { type: 'redirect'; to: string } | { type: 'wait' };

export type ResolvableSystemStatus = 'unknown' | 'checking' | 'initialized' | 'error';

export type ResolvableAuthStatus =
  | 'unknown'
  | 'rehydrating'
  | 'authenticated'
  | 'anonymous'
  | 'error';

export interface ResolveNavigationInput {
  systemStatus: string;
  authStatus: string;
  isAdmin: boolean;
  targetPath: string;
}

const LOGIN_PATH = '/login';
const REGISTER_PATH = '/register';
const HOME_PATH = '/';
const ADMIN_PREFIX = '/admin';

/**
 * Pure route-policy resolver.
 *
 * This function must stay deterministic and side-effect free.
 * It must not import stores, router, localStorage, or perform async work.
 *
 * Decision table:
 *
 * | System           | Auth                | Target              | Decision        |
 * | ---------------- | ------------------- | ------------------- | --------------- |
 * | unknown/checking | *                   | *                   | wait            |
 * | error            | *                   | *                   | wait            |
 * | initialized      | unknown/rehydrating | *                   | wait            |
 * | initialized      | error               | guest               | allow           |
 * | initialized      | error               | protected/admin     | redirect /login |
 * | initialized      | anonymous           | /login,/register    | allow           |
 * | initialized      | anonymous           | protected/admin     | redirect /login |
 * | initialized      | authenticated       | /login,/register    | redirect /      |
 * | initialized      | authenticated       | admin + isAdmin     | allow           |
 * | initialized      | authenticated       | admin + !isAdmin    | redirect /      |
 * | initialized      | authenticated       | protected           | allow           |
 */
export function resolveNavigationTarget(input: ResolveNavigationInput): RouteDecision {
  const systemStatus = normalizeSystemStatus(input.systemStatus);
  const authStatus = normalizeAuthStatus(input.authStatus);
  const targetPath = normalizeTargetPath(input.targetPath);
  const isAdmin = input.isAdmin === true;

  if (isSystemPending(systemStatus)) {
    return { type: 'wait' };
  }

  /**
   * Fail closed while system status is unavailable.
   *
   * The router guard decides how to surface this state. The resolver should not
   * redirect users to login for system failures because system health is not an
   * authentication signal.
   */
  if (systemStatus === 'error') {
    return { type: 'wait' };
  }

  if (systemStatus !== 'initialized') {
    return { type: 'wait' };
  }

  if (isAuthPending(authStatus)) {
    return { type: 'wait' };
  }

  const guestRoute = isGuestRoute(targetPath);
  const adminRoute = isAdminRoute(targetPath);

  if (authStatus === 'authenticated') {
    if (guestRoute) {
      return {
        type: 'redirect',
        to: HOME_PATH,
      };
    }

    if (adminRoute && !isAdmin) {
      return {
        type: 'redirect',
        to: HOME_PATH,
      };
    }

    return {
      type: 'allow',
    };
  }

  if (authStatus === 'anonymous') {
    if (guestRoute) {
      return {
        type: 'allow',
      };
    }

    return {
      type: 'redirect',
      to: LOGIN_PATH,
    };
  }

  /**
   * Auth error means the app could not prove the user is authenticated.
   * Guest routes remain usable. Protected routes fail closed to login.
   */
  if (authStatus === 'error') {
    if (guestRoute) {
      return {
        type: 'allow',
      };
    }

    return {
      type: 'redirect',
      to: LOGIN_PATH,
    };
  }

  return {
    type: 'wait',
  };
}

function normalizeSystemStatus(status: string): ResolvableSystemStatus | 'unsupported' {
  switch (status) {
    case 'unknown':
    case 'checking':
    case 'initialized':
    case 'error':
      return status;
    default:
      return 'unsupported';
  }
}

function normalizeAuthStatus(status: string): ResolvableAuthStatus | 'unsupported' {
  switch (status) {
    case 'unknown':
    case 'rehydrating':
    case 'authenticated':
    case 'anonymous':
    case 'error':
      return status;
    default:
      return 'unsupported';
  }
}

function normalizeTargetPath(path: string): string {
  if (!path) return HOME_PATH;

  if (!path.startsWith('/')) {
    return `/${path}`;
  }

  return path;
}

function isSystemPending(status: ResolvableSystemStatus | 'unsupported'): boolean {
  return status === 'unknown' || status === 'checking' || status === 'unsupported';
}

function isAuthPending(status: ResolvableAuthStatus | 'unsupported'): boolean {
  return status === 'unknown' || status === 'rehydrating' || status === 'unsupported';
}

function isGuestRoute(path: string): boolean {
  return path === LOGIN_PATH || path === REGISTER_PATH;
}

function isAdminRoute(path: string): boolean {
  return path === ADMIN_PREFIX || path.startsWith(`${ADMIN_PREFIX}/`);
}
