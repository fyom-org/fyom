/**
 * usePlaybackAdjustments — color adjustments + A/V delays composable.
 *
 * PORTED_FROM_SOIA @ <2025-Q4> (`src/composables/usePlaybackAdjustments.ts`, GPL-3.0-only)
 *
 * Adapted for fyom:
 * - soia's `useUiStateStore` (a persisted UI-state store backed by soia's Rust store) is
 *   replaced with **localStorage** (fyom doesn't have a Rust-side UI-state store; v1
 *   uses browser-local persistence for user preferences). The persistence shape is
 *   compatible with soia's JSON schema so a future migration to a Rust store is
 *   straightforward.
 * - soia's `invoke("mpv_set_option_string", { name, value })` is replaced with fyom's
 *   typed `setColorAdjustment(name, value)` / `setSubDelay(seconds)` / `setAudioDelay`
 *   bridges.
 * - Dual-sub support (`secondarySubDelay` + `setSubDelayForTarget`) is **deferred** to
 *   Phase 2.6+ (fyom v1 ships single-sub only). The `setSubDelay` method sets the
 *   primary subtitle delay; the secondary-sub API will be added when dual-sub lands.
 * - The per-media local color adjustments LRU cache (soia's
 *   `localColorAdjustmentsByMediaKey` Map, capped at 100 entries) is **ported verbatim**
 *   — it remembers color adjustments per media item (so the user's brightness tweak for
 *   a dark movie doesn't bleed into a bright sitcom).
 * - The global-vs-local color adjustments toggle is **ported verbatim** — when global is
 *   on, adjustments apply to all media; when off, adjustments are per-media.
 */

import { computed, ref } from 'vue';

import {
  setAudioDelay as bridgeSetAudioDelay,
  setColorAdjustment as bridgeSetColorAdjustment,
  setSubDelay as bridgeSetSubDelay,
  setSubScale as bridgeSetSubScale,
} from '@/lib/player/native-player';

const clamp = (value: number, min: number, max: number): number =>
  Math.min(max, Math.max(min, value));

type ColorAdjustmentKey = 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue';

type ColorAdjustmentsState = Record<ColorAdjustmentKey, number>;

interface PersistedPlaybackAdjustmentsState {
  globalColorAdjustmentsEnabled?: boolean;
  globalColorAdjustments?: Partial<ColorAdjustmentsState>;
  subScale?: number;
}

const COLOR_ADJUSTMENT_KEYS: ColorAdjustmentKey[] = [
  'brightness',
  'contrast',
  'saturation',
  'gamma',
  'hue',
];

const DEFAULT_COLOR_ADJUSTMENTS: ColorAdjustmentsState = {
  brightness: 0,
  contrast: 0,
  saturation: 0,
  gamma: 0,
  hue: 0,
};

const LOCAL_ADJUSTMENTS_MAX_ENTRIES = 100;
const STORAGE_KEY = 'fyom:playback-adjustments';

const normalizeColorAdjustmentValue = (value: unknown): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0;
  return clamp(Math.round(value), -100, 100);
};

const normalizeColorAdjustments = (
  values?: Partial<ColorAdjustmentsState>,
): ColorAdjustmentsState =>
  COLOR_ADJUSTMENT_KEYS.reduce<ColorAdjustmentsState>(
    (acc, key) => {
      const fallback = DEFAULT_COLOR_ADJUSTMENTS[key];
      acc[key] =
        values && key in values
          ? normalizeColorAdjustmentValue(values[key])
          : fallback;
      return acc;
    },
    { ...DEFAULT_COLOR_ADJUSTMENTS },
  );

/** Load persisted adjustments from localStorage (soia uses a Rust store; fyom uses
 *  localStorage for v1 — the JSON shape is compatible for future migration). */
const loadPersistedState = (): PersistedPlaybackAdjustmentsState => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as PersistedPlaybackAdjustmentsState;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
};

/** Save persisted adjustments to localStorage (debounced by the caller). */
const savePersistedState = (state: PersistedPlaybackAdjustmentsState): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    // Best-effort — ignore quota / serialization errors.
  }
};

export function usePlaybackAdjustments() {
  const showSettingsMenu = ref(false);
  const audioDelay = ref(0);
  const subDelay = ref(0);
  const subScale = ref(1.0);
  const localColorAdjustments = ref<ColorAdjustmentsState>({
    ...DEFAULT_COLOR_ADJUSTMENTS,
  });
  const localColorAdjustmentsByMediaKey = new Map<string, ColorAdjustmentsState>();
  const currentLocalMediaKey = ref('');
  const globalColorAdjustments = ref<ColorAdjustmentsState>({
    ...DEFAULT_COLOR_ADJUSTMENTS,
  });
  const globalColorAdjustmentsEnabled = ref(false);

  // Debounced persistence (350ms — same as soia).
  let persistTimer: ReturnType<typeof setTimeout> | null = null;
  const persistPlaybackAdjustmentsDebounced = (): void => {
    if (!globalColorAdjustmentsEnabled.value) return;
    if (persistTimer != null) clearTimeout(persistTimer);
    persistTimer = setTimeout(() => {
      savePersistedState({
        globalColorAdjustmentsEnabled: globalColorAdjustmentsEnabled.value,
        globalColorAdjustments: { ...globalColorAdjustments.value },
        subScale: subScale.value,
      });
    }, 350);
  };

  const persistPlaybackAdjustmentsNow = (): void => {
    if (persistTimer != null) clearTimeout(persistTimer);
    savePersistedState({
      globalColorAdjustmentsEnabled: globalColorAdjustmentsEnabled.value,
      globalColorAdjustments: { ...globalColorAdjustments.value },
      subScale: subScale.value,
    });
  };

  const activeColorAdjustments = computed<ColorAdjustmentsState>(() =>
    globalColorAdjustmentsEnabled.value
      ? globalColorAdjustments.value
      : localColorAdjustments.value,
  );

  const brightness = computed(() => activeColorAdjustments.value.brightness);
  const contrast = computed(() => activeColorAdjustments.value.contrast);
  const saturation = computed(() => activeColorAdjustments.value.saturation);
  const gamma = computed(() => activeColorAdjustments.value.gamma);
  const hue = computed(() => activeColorAdjustments.value.hue);

  const applyColorAdjustment = async (
    option: ColorAdjustmentKey,
    next: number,
  ): Promise<void> => {
    await bridgeSetColorAdjustment(option, next);
  };

  const reapplyGlobalColorAdjustments = async (): Promise<void> => {
    if (!globalColorAdjustmentsEnabled.value) return;
    await Promise.all(
      COLOR_ADJUSTMENT_KEYS.map((key) =>
        applyColorAdjustment(key, globalColorAdjustments.value[key]),
      ),
    );
  };

  const applyColorAdjustmentsSet = async (values: ColorAdjustmentsState): Promise<void> => {
    await Promise.all(
      COLOR_ADJUSTMENT_KEYS.map((key) => applyColorAdjustment(key, values[key])),
    );
  };

  const setColorAdjustment = async (key: ColorAdjustmentKey, value: number): Promise<void> => {
    const next = clamp(value, -100, 100);
    activeColorAdjustments.value[key] = next;
    if (!globalColorAdjustmentsEnabled.value && currentLocalMediaKey.value) {
      const mediaKey = currentLocalMediaKey.value;
      localColorAdjustmentsByMediaKey.delete(mediaKey);
      localColorAdjustmentsByMediaKey.set(mediaKey, {
        ...localColorAdjustments.value,
      });
      if (localColorAdjustmentsByMediaKey.size > LOCAL_ADJUSTMENTS_MAX_ENTRIES) {
        const oldestKey = localColorAdjustmentsByMediaKey.keys().next().value;
        if (oldestKey) {
          localColorAdjustmentsByMediaKey.delete(oldestKey);
        }
      }
    }
    await applyColorAdjustment(key, next);
    if (globalColorAdjustmentsEnabled.value) {
      persistPlaybackAdjustmentsDebounced();
    }
  };

  /** Load per-media adjustments (or global if global is enabled). Call on file-loaded. */
  const applyColorAdjustmentsForMedia = async (mediaKey: string): Promise<void> => {
    const normalizedKey = mediaKey.trim();
    currentLocalMediaKey.value = normalizedKey;
    if (globalColorAdjustmentsEnabled.value) {
      await reapplyGlobalColorAdjustments();
      return;
    }

    const storedPerMedia = normalizedKey
      ? localColorAdjustmentsByMediaKey.get(normalizedKey)
      : undefined;
    const perMedia = storedPerMedia ?? DEFAULT_COLOR_ADJUSTMENTS;
    if (normalizedKey && storedPerMedia) {
      localColorAdjustmentsByMediaKey.delete(normalizedKey);
      localColorAdjustmentsByMediaKey.set(normalizedKey, { ...storedPerMedia });
    }
    localColorAdjustments.value = { ...perMedia };
    await applyColorAdjustmentsSet(localColorAdjustments.value);
  };

  const setAudioDelay = async (value: number): Promise<void> => {
    const next = clamp(value, -5, 5);
    audioDelay.value = next;
    await bridgeSetAudioDelay(next);
  };

  const setSubDelay = async (value: number): Promise<void> => {
    const next = clamp(value, -10, 10);
    subDelay.value = next;
    await bridgeSetSubDelay(next);
  };

  const setSubScale = async (value: number): Promise<void> => {
    const next = clamp(value, 0.1, 10);
    subScale.value = next;
    await bridgeSetSubScale(next);
    persistPlaybackAdjustmentsDebounced();
  };

  const setBrightness = async (value: number): Promise<void> => {
    await setColorAdjustment('brightness', value);
  };
  const setContrast = async (value: number): Promise<void> => {
    await setColorAdjustment('contrast', value);
  };
  const setSaturation = async (value: number): Promise<void> => {
    await setColorAdjustment('saturation', value);
  };
  const setGamma = async (value: number): Promise<void> => {
    await setColorAdjustment('gamma', value);
  };
  const setHue = async (value: number): Promise<void> => {
    await setColorAdjustment('hue', value);
  };

  const setGlobalColorAdjustmentsEnabled = async (enabled: boolean): Promise<void> => {
    if (globalColorAdjustmentsEnabled.value === enabled) return;
    globalColorAdjustmentsEnabled.value = enabled;
    await applyColorAdjustmentsSet(activeColorAdjustments.value);
    persistPlaybackAdjustmentsNow();
  };

  /** Reconcile the refs with externally-observed mpv property values (from
   *  `fyom://mpv/*` events). Prevents feedback loops when the user drags a slider. */
  const reconcileFromMpv = (data: {
    brightness?: number;
    contrast?: number;
    saturation?: number;
    gamma?: number;
    hue?: number;
    subDelay?: number;
    audioDelay?: number;
  }): void => {
    if (typeof data.brightness === 'number') activeColorAdjustments.value.brightness = data.brightness;
    if (typeof data.contrast === 'number') activeColorAdjustments.value.contrast = data.contrast;
    if (typeof data.saturation === 'number') activeColorAdjustments.value.saturation = data.saturation;
    if (typeof data.gamma === 'number') activeColorAdjustments.value.gamma = data.gamma;
    if (typeof data.hue === 'number') activeColorAdjustments.value.hue = data.hue;
    if (typeof data.subDelay === 'number') subDelay.value = data.subDelay;
    if (typeof data.audioDelay === 'number') audioDelay.value = data.audioDelay;
  };

  // Load persisted state on composable creation (matches soia's IIFE pattern).
  void ((): void => {
    const persisted = loadPersistedState();
    const enabled = persisted.globalColorAdjustmentsEnabled === true;
    globalColorAdjustments.value = normalizeColorAdjustments(
      persisted.globalColorAdjustments,
    );
    globalColorAdjustmentsEnabled.value = enabled;
    if (typeof persisted.subScale === 'number') {
      subScale.value = clamp(persisted.subScale, 0.1, 10);
    }
    if (!enabled) return;
    void reapplyGlobalColorAdjustments();
  })();

  return {
    showSettingsMenu,
    audioDelay,
    subDelay,
    subScale,
    brightness,
    contrast,
    saturation,
    gamma,
    hue,
    globalColorAdjustmentsEnabled,
    setAudioDelay,
    setSubDelay,
    setSubScale,
    setBrightness,
    setContrast,
    setSaturation,
    setGamma,
    setHue,
    setGlobalColorAdjustmentsEnabled,
    reapplyGlobalColorAdjustments,
    applyColorAdjustmentsForMedia,
    reconcileFromMpv,
  };
}
