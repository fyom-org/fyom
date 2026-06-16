/**
 * State-matrix tests for resolveNavigationTarget.
 *
 * Exhaustively verifies the route policy decision table.
 */

import { describe, it, expect } from 'vitest';
import { resolveNavigationTarget } from '@/lib/navigation/resolveNavigationTarget';

describe('resolveNavigationTarget state matrix', () => {
  // 1. unknown/checking system → wait
  it('unknown system → wait', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'unknown', authStatus: 'unknown', isAdmin: false, targetPath: '/',
    })).toEqual({ type: 'wait' });
  });

  it('checking system → wait', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'checking', authStatus: 'authenticated', isAdmin: true, targetPath: '/admin/stats',
    })).toEqual({ type: 'wait' });
  });

  it('error system → wait', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'error', authStatus: 'authenticated', isAdmin: true, targetPath: '/',
    })).toEqual({ type: 'wait' });
  });

  // 2. initialized + unknown/rehydrating auth → wait
  it('initialized + unknown auth → wait', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'unknown', isAdmin: false, targetPath: '/',
    })).toEqual({ type: 'wait' });
  });

  it('initialized + rehydrating auth → wait', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'rehydrating', isAdmin: false, targetPath: '/library',
    })).toEqual({ type: 'wait' });
  });

  // 3. initialized + anonymous
  it('initialized + anonymous + /login → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/login',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + anonymous + /register → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/register',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + anonymous + /setup → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/setup',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + anonymous + / → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + anonymous + /profile → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/profile',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + anonymous + /library → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/library',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + anonymous + /admin/stats → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'anonymous', isAdmin: false, targetPath: '/admin/stats',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  // 4. initialized + authenticated + user role
  it('initialized + authenticated user + / → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated user + /library → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/library',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated user + /profile → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/profile',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated user + /setup → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/setup',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + authenticated user + /login → redirect /', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/login',
    })).toEqual({ type: 'redirect', to: '/' });
  });

  it('initialized + authenticated user + /register → redirect /', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/register',
    })).toEqual({ type: 'redirect', to: '/' });
  });

  it('initialized + authenticated user + /admin/stats → redirect /', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: false, targetPath: '/admin/stats',
    })).toEqual({ type: 'redirect', to: '/' });
  });

  // 5. initialized + authenticated + admin role
  it('initialized + authenticated admin + /admin/stats → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/admin/stats',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated admin + /admin/libraries → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/admin/libraries',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated admin + /profile → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/profile',
    })).toEqual({ type: 'allow' });
  });

  it('initialized + authenticated admin + /setup → redirect /login', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/setup',
    })).toEqual({ type: 'redirect', to: '/login' });
  });

  it('initialized + authenticated admin + /login → redirect /', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/login',
    })).toEqual({ type: 'redirect', to: '/' });
  });

  it('initialized + authenticated admin + / → allow', () => {
    expect(resolveNavigationTarget({
      systemStatus: 'initialized', authStatus: 'authenticated', isAdmin: true, targetPath: '/',
    })).toEqual({ type: 'allow' });
  });
});
