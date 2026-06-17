/**
 * Component-level tests for PlayerView native fallback behavior.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import type { MediaItem } from '@/api/library';

vi.mock('@/api/library', () => ({
  getMediaDetail: vi.fn(),
}));

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'test-123' } }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

vi.mock('@/lib/runtime/tauri', () => ({
  isTauriEnvironment: vi.fn().mockReturnValue(false),
}));

import { getMediaDetail } from '@/api/library';
import { isTauriEnvironment } from '@/lib/runtime/tauri';

const mockGetMediaDetail = vi.mocked(getMediaDetail);
const mockIsTauri = vi.mocked(isTauriEnvironment);

type TauriInvokeMock = ReturnType<typeof vi.fn>;

function setMockTauriInternals(invokeMock: TauriInvokeMock): void {
  (window as any).__TAURI_INTERNALS__ = {
    tauri: {
      invoke: invokeMock,
    },
  };
}

function clearMockTauriInternals(): void {
  delete (window as any).__TAURI_INTERNALS__;
}

function makeMediaDetail(streamUrl: string | null): MediaItem {
  return {
    id: 'test-123',
    type: 'movie',
    title: 'Test Media',
    stream_url: streamUrl,
    library_id: 'lib-1',
    provider_id: 'local',
    created_at: '2026-06-17T00:00:00Z',
    updated_at: '2026-06-17T00:00:00Z',
  };
}

async function importPlayerView() {
  const module = await import('@/views/PlayerView.vue');
  return module.default;
}

async function mountPlayer(
  streamUrl: string | null = 'http://test/video.mkv',
  tauriEnv = false,
  invokeMock?: TauriInvokeMock
): Promise<VueWrapper<any>> {
  mockGetMediaDetail.mockResolvedValue(makeMediaDetail(streamUrl));
  mockIsTauri.mockReturnValue(tauriEnv);

  if (invokeMock) {
    setMockTauriInternals(invokeMock);
  } else {
    clearMockTauriInternals();
  }

  const PlayerView = await importPlayerView();

  return mount(PlayerView, {
    global: {
      stubs: {
        RouterLink: true,
        PlayerFallbackNotice: {
          props: ['message'],
          template: '<div class="fallback-notice">{{ message }}</div>',
        },
      },
    },
  });
}

async function settlePlayer(): Promise<void> {
  await nextTick();
  await nextTick();
  await new Promise((resolve) => setTimeout(resolve, 200));
  await nextTick();
  await nextTick();
}

function getVideoSrc(wrapper: VueWrapper<any>): string | undefined {
  const video = wrapper.find('video.video-player');
  if (!video.exists()) return undefined;

  const el = video.element as HTMLVideoElement;
  return el.getAttribute('src') ?? undefined;
}

describe('PlayerView native fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    clearMockTauriInternals();
    mockIsTauri.mockReturnValue(false);
  });

  afterEach(() => {
    clearMockTauriInternals();
    mockIsTauri.mockReturnValue(false);
  });

  it('renders browser playback by default outside native runtime', async () => {
    const wrapper = await mountPlayer('http://test/video.mkv', false);
    await settlePlayer();

    expect(mockGetMediaDetail).toHaveBeenCalledWith('test-123');
    expect(wrapper.find('video.video-player').exists()).toBe(true);
    expect(getVideoSrc(wrapper)).toBe('http://test/video.mkv');
    expect(wrapper.text()).not.toContain('Native player unavailable');
    expect(wrapper.find('.spinner').exists()).toBe(false);
  });

  it('renders loading state while native player initializes', async () => {
    mockIsTauri.mockReturnValue(true);

    const pendingPromise = new Promise(() => {});
    const mockInvoke = vi.fn().mockReturnValue(pendingPromise);
    setMockTauriInternals(mockInvoke);

    const wrapper = await mountPlayer('http://test/video.mkv', true, mockInvoke);
    await nextTick();

    expect(wrapper.find('.spinner').exists()).toBe(true);
    expect(wrapper.text()).toContain('Initializing native player');
    expect(wrapper.text()).not.toContain('Native player unavailable');
    expect(wrapper.find('video.video-player').exists()).toBe(false);
    expect(mockInvoke).toHaveBeenCalledTimes(1);
  });

  it('falls back to browser playback when native init fails', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockRejectedValue(new Error('mpv init failed'));
    setMockTauriInternals(mockInvoke);

    const wrapper = await mountPlayer('http://test/video.mkv', true, mockInvoke);
    await settlePlayer();

    expect(wrapper.find('video.video-player').exists()).toBe(true);
    expect(getVideoSrc(wrapper)).toBe('http://test/video.mkv');
    expect(wrapper.text()).toContain('Native player unavailable, using browser playback');
    expect(wrapper.find('.spinner').exists()).toBe(false);
    expect(mockInvoke).toHaveBeenCalledTimes(1);
  });

  it('renders native surface when native init succeeds', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockResolvedValue({ success: true });
    setMockTauriInternals(mockInvoke);

    const wrapper = await mountPlayer('http://test/video.mkv', true, mockInvoke);
    await settlePlayer();

    expect(wrapper.find('video.video-player').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Native player unavailable');
    expect(wrapper.find('.spinner').exists()).toBe(false);
    expect(mockInvoke).toHaveBeenCalledTimes(1);
  });

  it('does not retry native init endlessly after failure within one mount lifecycle', async () => {
    mockIsTauri.mockReturnValue(true);

    const mockInvoke = vi.fn().mockRejectedValue(new Error('mpv init failed'));
    setMockTauriInternals(mockInvoke);

    const wrapper = await mountPlayer('http://test/video.mkv', true, mockInvoke);
    await settlePlayer();

    expect(mockInvoke).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain('Native player unavailable, using browser playback');

    await wrapper.setProps({});
    await nextTick();
    await nextTick();

    expect(mockInvoke).toHaveBeenCalledTimes(1);
  });

  it('renders browser player when stream_url is missing', async () => {
    const wrapper = await mountPlayer(null, false);
    await settlePlayer();

    expect(wrapper.find('video.video-player').exists()).toBe(true);
    expect(getVideoSrc(wrapper)).toBeUndefined();
    expect(wrapper.find('.spinner').exists()).toBe(false);
  });
});
