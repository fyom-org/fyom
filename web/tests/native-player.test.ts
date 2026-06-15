/**
 * Unit tests for native player state model.
 *
 * Run with: npx vitest run web/tests/native-player.test.ts
 * (requires vitest to be installed: pnpm add -D vitest)
 */

import { describe, it, expect } from 'vitest';
import {
  createInitialNativePlayerState,
  mapNativePlayerInitError,
  isNativePlayerAvailable,
  type NativePlayerState,
} from '@/lib/player/native-player';

describe('createInitialNativePlayerState', () => {
  it('creates idle state with no failure', () => {
    const state = createInitialNativePlayerState();
    expect(state.status).toBe('idle');
    expect(state.failure).toBeNull();
  });
});

describe('mapNativePlayerInitError', () => {
  it('maps raw-window-handle errors', () => {
    const err = new Error('Failed to get RawWindowHandle');
    const failure = mapNativePlayerInitError(err);
    expect(failure.stage).toBe('raw-window-handle');
    expect(failure.message).toBe('Failed to get RawWindowHandle');
  });

  it('maps wid-injection errors', () => {
    const err = new Error('WID injection failed');
    const failure = mapNativePlayerInitError(err);
    expect(failure.stage).toBe('wid-injection');
  });

  it('maps mpv-context errors', () => {
    const err = new Error('mpv_context_create failed');
    const failure = mapNativePlayerInitError(err);
    expect(failure.stage).toBe('mpv-context');
  });

  it('maps library-load errors', () => {
    const err = new Error('Failed to load libmpv.dylib');
    const failure = mapNativePlayerInitError(err);
    expect(failure.stage).toBe('library-load');
  });

  it('maps unknown errors', () => {
    const err = new Error('Something unexpected happened');
    const failure = mapNativePlayerInitError(err);
    expect(failure.stage).toBe('unknown');
  });

  it('handles non-Error values', () => {
    const failure = mapNativePlayerInitError('string error');
    expect(failure.stage).toBe('unknown');
    expect(failure.message).toBe('string error');
  });
});

describe('isNativePlayerAvailable', () => {
  it('returns false in non-Tauri environment', () => {
    // In test environment, __TAURI_INTERNALS__ is not defined
    expect(isNativePlayerAvailable()).toBe(false);
  });
});
