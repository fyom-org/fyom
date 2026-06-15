/**
 * Component-level tests for PlayerView native fallback behavior.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';

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

async function mountPlayer(
  streamUrl: string | null = 'http://test/video.mkv',
  tauriEnv = false,
  invokeMock?: ReturnType<typeof vi.fn>,
) {
  mockGetMediaDetail.mockResolvedValue({ stream_url: streamUrl });
  mockIsTauri.mockReturnValue(tauriEnv);

  if (invokeMock) {
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: invokeMock } };
  }

  const PlayerView = (await import('@/views/PlayerView.vue')).default;

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

function getVideoSrc(wrapper: ReturnType<typeof mount>): string | undefined {
  const el = wrapper.find('video.video-player').element as HTMLVideoElement | undefined;
  return el?.getAttribute('src') ?? undefined;
}

describe('PlayerView native fallback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // @ts-expect-error - cleanup
    delete window.__TAURI_INTERNALS__;
    // Reset to default browser mode after clearAllMocks
    mockIsTauri.mockReturnValue(false);
  });

  afterEach(() => {
    mockIsTauri.mockReturnValue(false);
    // @ts-expect-error - cleanup
    delete window.__TAURI_INTERNALS__;
  });

  it('renders browser playback by default outside native runtime', async () => {
    const wrapper = await mountPlayer('http://test/video.mkv', false);
    await nextTick();
    await nextTick();
    await nextTick();

    expect(wrapper.find('video.video-player').exists()).toBe(true);
    expect(getVideoSrc(wrapper)).toBe('http://test/video.mkv');
    expect(wrapper.text()).not.toContain('Native player unavailable');
    expect(wrapper.find('.spinner').exists()).toBe(false);
  });

  it('renders loading state while native player initializes', async () => {
    mockIsTauri.mockReturnValue(true);

    const pendingPromise = new Promise(() => {});
    const mockInvoke = vi.fn().mockReturnValue(pendingPromise);
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const wrapper = await mountPlayer('http://test/video.mkv', true);
    await nextTick();

    expect(wrapper.find('.spinner').exists()).toBe(true);
    expect(wrapper.text()).toContain('Initializing native player');
    expect(wrapper.text()).not.toContain('Native player unavailable');
    expect(wrapper.find('video.video-player').exists()).toBe(false);
  });

  it('falls back to browser playback when native init fails', async () => {
    mockIsTauri.mockReturnValue(true);
    const mockInvoke = vi.fn().mockRejectedValue(new Error('mpv init failed'));
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const wrapper = await mountPlayer('http://test/video.mkv', true);
    await nextTick();
    await nextTick();
    await new Promise((r) => setTimeout(r, 200));
    await nextTick();
    await nextTick();

    expect(wrapper.find('video.video-player').exists()).toBe(true);
    expect(wrapper.text()).toContain('Native player unavailable, using browser playback');
    expect(wrapper.find('.spinner').exists()).toBe(false);
  });

  it('renders native surface when native init succeeds', async () => {
    mockIsTauri.mockReturnValue(true);
    const mockInvoke = vi.fn().mockResolvedValue({ success: true });
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const wrapper = await mountPlayer('http://test/video.mkv', true);
    await nextTick();
    await nextTick();
    await new Promise((r) => setTimeout(r, 200));
    await nextTick();
    await nextTick();

    expect(wrapper.find('video.video-player').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Native player unavailable');
  });

  it('does not retry native init endlessly after failure within one mount lifecycle', async () => {
    mockIsTauri.mockReturnValue(true);
    const mockInvoke = vi.fn().mockRejectedValue(new Error('mpv init failed'));
    // @ts-expect-error - mocking window global
    window.__TAURI_INTERNALS__ = { tauri: { invoke: mockInvoke } };

    const wrapper = await mountPlayer('http://test/video.mkv', true);
    await nextTick();
    await nextTick();
    await new Promise((r) => setTimeout(r, 200));
    await nextTick();
    await nextTick();

    expect(mockInvoke).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain('Native player unavailable, using browser playback');

    await wrapper.setProps({});
    await nextTick();
    await nextTick();

    expect(mockInvoke).toHaveBeenCalledTimes(1);
  });
});
