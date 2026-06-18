/**
 * usePlaybackSpeed — playback speed control composable.
 *
 * PORTED_FROM_SOIA @ <2025-Q4> (`src/composables/usePlaybackSpeed.ts`, GPL-3.0-only)
 *
 * Adapted for fyom:
 * - soia's `invoke("mpv_set_option_string", { name: "speed", value: rate })` is
 *   replaced with fyom's typed `setSpeed(rate)` bridge (which invokes the `set_speed`
 *   Tauri command — same underlying `mpv_set_property("speed", rate)` call).
 * - The `currentSpeed` ref is updated optimistically (soia's pattern); the actual mpv
 *   state arrives via the `fyom://mpv/speed` event, which PlayerView can wire to
 *   reconcile the value (in case another control surface changed it).
 * - No-op outside the Tauri runtime (the `<video>` fallback owns playback).
 */

import { ref } from 'vue';

import { setSpeed } from '@/lib/player/native-player';

export const PLAYBACK_RATES = [0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0] as const;

export function usePlaybackSpeed() {
  const currentSpeed = ref<number>(1.0);
  const showSpeedMenu = ref<boolean>(false);

  /**
   * Set the playback speed. Updates the ref optimistically; the actual mpv state will be
   * confirmed via the `fyom://mpv/speed` event (PlayerView can wire a watcher to
   * reconcile if mpv rejects the value).
   */
  async function applySpeed(rate: number): Promise<void> {
    currentSpeed.value = rate;
    showSpeedMenu.value = false;
    await setSpeed(rate);
  }

  /** Toggle the speed picker menu visibility. */
  function toggleSpeedMenu(value?: boolean): void {
    showSpeedMenu.value = typeof value === 'boolean' ? value : !showSpeedMenu.value;
  }

  /** Reconcile the ref with an externally-observed speed (e.g. from `fyom://mpv/speed`). */
  function reconcileSpeed(rate: number): void {
    if (Number.isFinite(rate) && rate > 0) {
      currentSpeed.value = rate;
    }
  }

  return {
    playbackRates: PLAYBACK_RATES,
    currentSpeed,
    showSpeedMenu,
    setSpeed: applySpeed,
    toggleSpeedMenu,
    reconcileSpeed,
  };
}
