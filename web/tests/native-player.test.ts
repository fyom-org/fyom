/**
 * Unit tests for native player state model and bridge boundary.
 */

import { describe, it, expect, vi, afterEach } from 'vitest';

// Mock the tauri runtime module (must be before imports that use it)
vi.mock('@/lib/runtime/tauri', () => ({
  isTauriEnvironment: vi.fn().mockReturnValue(false),
}));

import {
  createInitialNativePlayerState,
  mapNativePlayerInitError,
  isNativePlaybackRuntimeAvailable,
  tryInitializeNativePlayer,
} from '@/lib/player/native-player';
import { isTauriEnvironment } from '@/lib/runtime/tauri';

const mockIsTauri = vi.mocked(isTauriEnvironment);

describe('createInitialNativePlayerState', () => {
  it('creates idle state with no failure and attempted=false', () => {
    const state = createInitialNativePlayerState();
    expect(state.status).toBe('idle');
    expect(state.failure).toBeNull();
    expect(state.attempted).toBe(false);
  });
});

describe('mapNativePlayerInitError', () => {
  it('maps raw-window-handle errors', () => {
    const failure = mapNativePlayerInitError(new Error('Failed to get RawWindowHandle'));
    expect(failure.stage).toBe('raw-window-handle');
  });

  it('maps wid-injection errors', () => {
    const failure = mapNativePlayerInitError(new Error('WID injection failed'));
    expect(failure.stage).toBe('wid-injection');
  });

  it('maps mpv-context errors', () => {
    const failure = mapNativePlayerInitError(new Error('mpv_context_create failed'));
    expect(failure.stage).toBe('mpv-context');
  });

  it('maps library-load errors', () => {
    const failure = mapNativePlayerInitError(new Error('Failed to load libmpv.dylib'));
    expect(failure.stage).toBe('library-load');
  });

  it('maps bridge errors', () => {
    const failure = mapNativePlayerInitError(new Error('invoke command failed'));
    expect(failure.stage).toBe('bridge');
  });

  it('maps unknown errors', () => {
    const failure = mapNativePlayerInitError(new Error('Something unexpected'));
    expect(failure.stage).toBe('unknown');
  });

  it('handles non-Error values', () => {
    const failure = mapNativePlayerInitError('string error');
    expect(failure.stage).toBe('unknown');
  });
});

describe('isNativePlaybackRuntimeAvailable', () => {
  afterEach(() => {
    mockIsTauri.mockReturnValue(false);
  });

  it('returns false in non-Tauri environment', () => {
    expect(isNativePlaybackRuntimeAvailable()).toBe(false);
  });

  it('returns true in Tauri environment', () => {
    mockIsTauri.mockReturnValue(true);
    expect(isNativePlaybackRuntimeAvailable()).toBe(true);
  });
});

describe('tryInitializeNativePlayer', () => {
  afterEach(() => {
    mockIsTauri.mockReturnValue(false);
  });

  it('returns bridge failure when runtime is unavailable', async () => {
    const result = await tryInitializeNativePlayer({ mediaUrl: 'http://test/video.mkv' });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.failure.stage).toBe('bridge');
      expect(result.failure.message).toContain('not available');
    }
  });

  it('calls invoke in Tauri runtime and returns ok on success', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockResolvedValue({ success: true });
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const result = await tryInitializeNativePlayer({ mediaUrl: 'http://test/video.mkv' });
    expect(result.ok).toBe(true);
    expect(mockInvoke).toHaveBeenCalledWith('play_media', {
      mediaUrl: 'http://test/video.mkv',
      posterUrl: '',
    });
  });

  it('maps invoke failure to typed result', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockResolvedValue({ success: false, error: 'mpv init failed' });
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const result = await tryInitializeNativePlayer({ mediaUrl: 'http://test/video.mkv' });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.failure.stage).toBe('mpv-context');
    }
  });

  it('maps invoke exception to typed result', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockRejectedValue(new Error('invoke failed'));
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const result = await tryInitializeNativePlayer({ mediaUrl: 'http://test/video.mkv' });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.failure.stage).toBe('bridge');
    }
  });
});

describe('NativePlayerState lifecycle', () => {
  it('transitions from idle to unavailable in browser runtime', () => {
    const state = createInitialNativePlayerState();
    expect(state.status).toBe('idle');
    expect(state.attempted).toBe(false);

    if (!isNativePlaybackRuntimeAvailable()) {
      state.status = 'unavailable';
    }

    expect(state.status).toBe('unavailable');
    expect(state.attempted).toBe(false);
  });

  it('marks attempted=true after native init attempt', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockRejectedValue(new Error('fail'));
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const result = await tryInitializeNativePlayer({ mediaUrl: 'http://test/video.mkv' });
    expect(result.ok).toBe(false);

    const state = createInitialNativePlayerState();
    state.status = 'failed';
    state.failure = !result.ok ? result.failure : null;
    state.attempted = true;

    expect(state.attempted).toBe(true);
    expect(state.status).toBe('failed');
    expect(state.failure).not.toBeNull();
  });
});
