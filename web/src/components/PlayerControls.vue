<template>
  <!--
    Phase 2.4 PlayerControls — HTML controls overlay on top of the transparent webview.

    When `.video-mode` is active (Phase 2.3), the webview root is transparent so the mpv
    GL layer (rendered behind the webview by `attach_render_surface`) shows through. This
    component renders opaque HTML controls on top of the video — play/pause, scrubber,
    volume, speed, audio/subtitle pickers, settings menu (color adjustments + A/V delays),
    chapter nav, fullscreen.

    The controls auto-hide after 3s of mouse inactivity (ported from soia's
    `useHoverState` pattern, simplified). Move the mouse to reveal them again.

    All control actions emit events to the parent (PlayerView), which calls the
    `native-player.ts` bridge functions. The parent owns the playback state; this
    component is a pure presentational + emit layer.
  -->
  <div
    class="player-controls"
    :class="{ 'controls-visible': isVisible, 'controls-hidden': !isVisible }"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
  >
    <!-- Top bar: chapter + hwdec + stream info -->
    <div v-if="isVisible" class="controls-top-bar">
      <div class="top-left">
        <button
          v-if="chapters.length > 0 && currentChapter > 0"
          type="button"
          class="icon-btn"
          :title="$t('player.previousChapter')"
          @click="emit('set-chapter', currentChapter - 1)"
        >
          ⏮
        </button>
        <span v-if="chapters.length > 0" class="chapter-label">
          {{ currentChapterLabel }}
        </span>
        <button
          v-if="chapters.length > 0 && currentChapter < chapters.length - 1"
          type="button"
          class="icon-btn"
          :title="$t('player.nextChapter')"
          @click="emit('set-chapter', currentChapter + 1)"
        >
          ⏭
        </button>
      </div>
      <div class="top-right">
        <span v-if="state.hwdec" class="hwdec-badge" :title="$t('player.hwdecActive')">
          {{ state.hwdec }}
        </span>
      </div>
    </div>

    <!-- Center play/pause (big) — shows on click anywhere in the player area -->
    <button
      v-if="isVisible"
      type="button"
      class="center-play-btn"
      :title="state.isPaused ? $t('player.play') : $t('player.pause')"
      @click="emit('toggle-pause')"
    >
      <span v-if="state.isPaused">▶</span>
      <span v-else>⏸</span>
    </button>

    <!-- Bottom bar: scrubber + buttons -->
    <div v-if="isVisible" class="controls-bottom-bar">
      <!-- Scrubber -->
      <div class="scrubber-row">
        <span class="time-display">{{ formatTime(state.currentTime) }}</span>
        <input
          type="range"
          class="scrubber"
          min="0"
          :max="state.duration || 0"
          step="0.1"
          :value="scrubValue"
          :disabled="state.duration <= 0"
          :aria-label="$t('player.seek')"
          @input="onScrubInput"
          @change="onScrubChange"
        />
        <span class="time-display">{{ formatTime(state.duration) }}</span>
      </div>

      <!-- Buttons row -->
      <div class="buttons-row">
        <div class="buttons-left">
          <button
            type="button"
            class="icon-btn"
            :title="state.isPaused ? $t('player.play') : $t('player.pause')"
            @click="emit('toggle-pause')"
          >
            <span v-if="state.isPaused">▶</span>
            <span v-else>⏸</span>
          </button>
          <button
            type="button"
            class="icon-btn"
            :title="$t('player.seekBackward')"
            @click="emit('seek-relative', -10)"
          >
            ⏪
          </button>
          <button
            type="button"
            class="icon-btn"
            :title="$t('player.seekForward')"
            @click="emit('seek-relative', 10)"
          >
            ⏩
          </button>

          <!-- Volume -->
          <div class="volume-group">
            <button
              type="button"
              class="icon-btn"
              :title="$t('player.mute')"
              @click="toggleMute"
            >
              <span v-if="state.volume === 0">🔇</span>
              <span v-else-if="state.volume < 50">🔉</span>
              <span v-else>🔊</span>
            </button>
            <input
              type="range"
              class="volume-slider"
              min="0"
              max="100"
              step="1"
              :value="state.volume"
              :aria-label="$t('player.volume')"
              @input="onVolumeInput"
                />
          </div>

          <span class="time-display time-separator">{{ formatTime(state.currentTime) }} / {{ formatTime(state.duration) }}</span>
        </div>

        <div class="buttons-right">
          <!-- Speed picker -->
          <div class="menu-group">
            <button
              type="button"
              class="icon-btn"
              :title="$t('player.speed')"
              @click="toggleMenu('speed')"
            >
              {{ state.speed }}×
            </button>
            <div v-if="openMenu === 'speed'" class="menu-panel" @mouseleave="closeMenu">
              <button
                v-for="rate in PLAYBACK_RATES"
                :key="rate"
                type="button"
                class="menu-item"
                :class="{ active: state.speed === rate }"
                @click="emit('set-speed', rate); closeMenu()"
              >
                {{ rate }}×
              </button>
            </div>
          </div>

          <!-- Audio track picker -->
          <div class="menu-group">
            <button
              type="button"
              class="icon-btn"
              :title="$t('player.audioTracks')"
              :disabled="state.audioTracks.length === 0"
              @click="toggleMenu('audio')"
            >
              ♪
            </button>
            <div v-if="openMenu === 'audio'" class="menu-panel" @mouseleave="closeMenu">
              <div class="menu-header">{{ $t('player.audioTracks') }}</div>
              <button
                v-for="track in state.audioTracks"
                :key="track.id"
                type="button"
                class="menu-item"
                :class="{ active: track.id === state.currentAudioId }"
                @click="emit('select-audio', track.id); closeMenu()"
              >
                <span class="track-title">{{ track.title || `Track ${track.id}` }}</span>
                <span v-if="track.lang" class="track-lang">{{ track.lang }}</span>
              </button>
            </div>
          </div>

          <!-- Subtitle picker -->
          <div class="menu-group">
            <button
              type="button"
              class="icon-btn"
              :title="$t('player.subtitles')"
              @click="toggleMenu('sub')"
            >
              CC
            </button>
            <div v-if="openMenu === 'sub'" class="menu-panel" @mouseleave="closeMenu">
              <div class="menu-header">{{ $t('player.subtitles') }}</div>
              <button
                v-for="track in state.subTracks"
                :key="track.id"
                type="button"
                class="menu-item"
                :class="{ active: track.id === state.currentSubId }"
                @click="emit('select-sub', track.id); closeMenu()"
              >
                <span class="track-title">{{ track.title || (track.id === 0 ? $t('player.subtitlesOff') : `Track ${track.id}`) }}</span>
                <span v-if="track.lang" class="track-lang">{{ track.lang }}</span>
              </button>
            </div>
          </div>

          <!-- Settings (color adjustments + A/V delays) -->
          <div class="menu-group">
            <button
              type="button"
              class="icon-btn"
              :title="$t('player.settings')"
              @click="toggleMenu('settings')"
            >
              ⚙
            </button>
            <div v-if="openMenu === 'settings'" class="menu-panel settings-panel" @mouseleave="closeMenu">
              <div class="menu-header">{{ $t('player.settings') }}</div>

              <label class="settings-row">
                <span class="settings-label">{{ $t('player.brightness') }}</span>
                <input
                  type="range"
                  min="-100"
                  max="100"
                  step="1"
                  :value="state.brightness"
                  @input="onAdjustmentInput('brightness', $event)"
                />
                <span class="settings-value">{{ state.brightness }}</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.contrast') }}</span>
                <input
                  type="range"
                  min="-100"
                  max="100"
                  step="1"
                  :value="state.contrast"
                  @input="onAdjustmentInput('contrast', $event)"
                />
                <span class="settings-value">{{ state.contrast }}</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.saturation') }}</span>
                <input
                  type="range"
                  min="-100"
                  max="100"
                  step="1"
                  :value="state.saturation"
                  @input="onAdjustmentInput('saturation', $event)"
                />
                <span class="settings-value">{{ state.saturation }}</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.gamma') }}</span>
                <input
                  type="range"
                  min="-100"
                  max="100"
                  step="1"
                  :value="state.gamma"
                  @input="onAdjustmentInput('gamma', $event)"
                />
                <span class="settings-value">{{ state.gamma }}</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.hue') }}</span>
                <input
                  type="range"
                  min="-100"
                  max="100"
                  step="1"
                  :value="state.hue"
                  @input="onAdjustmentInput('hue', $event)"
                />
                <span class="settings-value">{{ state.hue }}</span>
              </label>

              <div class="settings-divider"></div>

              <label class="settings-row">
                <span class="settings-label">{{ $t('player.subDelay') }}</span>
                <input
                  type="range"
                  min="-10"
                  max="10"
                  step="0.1"
                  :value="state.subDelay"
                  @input="onSubDelayInput($event)"
                />
                <span class="settings-value">{{ state.subDelay.toFixed(1) }}s</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.audioDelay') }}</span>
                <input
                  type="range"
                  min="-5"
                  max="5"
                  step="0.1"
                  :value="state.audioDelay"
                  @input="onAudioDelayInput($event)"
                />
                <span class="settings-value">{{ state.audioDelay.toFixed(1) }}s</span>
              </label>
              <label class="settings-row">
                <span class="settings-label">{{ $t('player.subScale') }}</span>
                <input
                  type="range"
                  min="0.5"
                  max="3"
                  step="0.1"
                  :value="state.subScale"
                  @input="onSubScaleInput($event)"
                />
                <span class="settings-value">{{ state.subScale.toFixed(1) }}×</span>
              </label>

              <div class="settings-divider"></div>

              <label class="settings-row settings-checkbox-row">
                <input
                  type="checkbox"
                  :checked="state.globalColorAdjustmentsEnabled"
                  @change="onGlobalColorToggle($event)"
                />
                <span class="settings-label">{{ $t('player.globalColorAdjustments') }}</span>
              </label>

              <div class="settings-divider"></div>

              <button
                type="button"
                class="menu-item reset-btn"
                @click="emit('reset-adjustments'); closeMenu()"
              >
                {{ $t('player.resetAdjustments') }}
              </button>
            </div>
          </div>

          <!-- Fullscreen -->
          <button
            type="button"
            class="icon-btn"
            :title="$t('player.fullscreen')"
            @click="emit('toggle-fullscreen')"
          >
            ⛶
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';

import { PLAYBACK_RATES } from '@/composables/usePlaybackSpeed';
import type { FyomMediaTrack } from '@/composables/useMediaTracks';
import type { MpvChapter } from '@/lib/player/native-player';

/** The full playback state rendered by the controls. Owned by the parent (PlayerView). */
export interface PlayerControlsState {
  isPaused: boolean;
  currentTime: number;
  duration: number;
  volume: number;
  speed: number;
  audioTracks: FyomMediaTrack[];
  subTracks: FyomMediaTrack[];
  currentAudioId: number;
  currentSubId: number;
  chapters: MpvChapter[];
  currentChapter: number;
  hwdec: string;
  brightness: number;
  contrast: number;
  saturation: number;
  gamma: number;
  hue: number;
  subDelay: number;
  audioDelay: number;
  subScale: number;
  globalColorAdjustmentsEnabled: boolean;
  isBuffering: boolean;
}

const props = defineProps<{
  state: PlayerControlsState;
}>();

const emit = defineEmits<{
  'toggle-pause': [];
  seek: [position: number];
  'seek-relative': [delta: number];
  'set-volume': [volume: number];
  'set-speed': [speed: number];
  'select-audio': [trackId: number];
  'select-sub': [trackId: number];
  'set-color-adjustment': [name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue', value: number];
  'set-sub-delay': [seconds: number];
  'set-audio-delay': [seconds: number];
  'set-sub-scale': [scale: number];
  'set-global-color': [enabled: boolean];
  'set-chapter': [index: number];
  'toggle-fullscreen': [];
  'reset-adjustments': [];
}>();

// Auto-hide controls after 3s of mouse inactivity.
const CONTROLS_HIDE_DELAY_MS = 3000;
const isVisible = ref(true);
let hideTimer: ReturnType<typeof setTimeout> | null = null;

const scheduleHide = (): void => {
  if (hideTimer != null) clearTimeout(hideTimer);
  hideTimer = setTimeout(() => {
    isVisible.value = false;
    closeMenu();
  }, CONTROLS_HIDE_DELAY_MS);
};

const reveal = (): void => {
  isVisible.value = true;
  scheduleHide();
};

const onMouseMove = (): void => {
  reveal();
};

const onMouseLeave = (): void => {
  if (hideTimer != null) clearTimeout(hideTimer);
  isVisible.value = false;
  closeMenu();
};

// Menu state (speed / audio / sub / settings — only one open at a time).
type MenuName = 'speed' | 'audio' | 'sub' | 'settings' | null;
const openMenu = ref<MenuName>(null);

const toggleMenu = (name: MenuName): void => {
  openMenu.value = openMenu.value === name ? null : name;
  reveal();
};

const closeMenu = (): void => {
  openMenu.value = null;
};

// Scrubber — local value while dragging (so the thumb follows the mouse before mpv
// confirms the seek).
const isScrubbing = ref(false);
const localScrubValue = ref(0);

const scrubValue = computed<number>(() => {
  if (isScrubbing.value) return localScrubValue.value;
  return props.state.currentTime;
});

const onScrubInput = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  if (!Number.isFinite(value)) return;
  isScrubbing.value = true;
  localScrubValue.value = value;
};

const onScrubChange = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  isScrubbing.value = false;
  if (Number.isFinite(value)) {
    emit('seek', value);
  }
};

// Volume.
const onVolumeInput = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseInt(target.value, 10);
  if (Number.isFinite(value)) {
    emit('set-volume', value);
  }
  reveal();
};

const previousVolume = ref(80);
const toggleMute = (): void => {
  if (props.state.volume > 0) {
    previousVolume.value = props.state.volume;
    emit('set-volume', 0);
  } else {
    emit('set-volume', previousVolume.value || 80);
  }
};

// Color adjustments.
const onAdjustmentInput = (
  name: 'brightness' | 'contrast' | 'saturation' | 'gamma' | 'hue',
  event: Event,
): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  if (Number.isFinite(value)) {
    emit('set-color-adjustment', name, value);
  }
};

const onSubDelayInput = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  if (Number.isFinite(value)) {
    emit('set-sub-delay', value);
  }
};

const onAudioDelayInput = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  if (Number.isFinite(value)) {
    emit('set-audio-delay', value);
  }
};

const onSubScaleInput = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  const value = parseFloat(target.value);
  if (Number.isFinite(value)) {
    emit('set-sub-scale', value);
  }
};

const onGlobalColorToggle = (event: Event): void => {
  const target = event.target as HTMLInputElement;
  emit('set-global-color', target.checked);
};

// Chapter label.
const currentChapterLabel = computed<string>(() => {
  const idx = props.state.currentChapter;
  if (idx < 0 || idx >= props.state.chapters.length) return '';
  const ch = props.state.chapters[idx];
  return ch.title || `${idx + 1} / ${props.state.chapters.length}`;
});

// Time formatting (HH:MM:SS for long content, MM:SS for short).
const formatTime = (seconds: number): string => {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${m}:${String(s).padStart(2, '0')}`;
};

// Reveal controls when pause state changes (so the user sees feedback).
watch(
  () => props.state.isPaused,
  () => reveal(),
);

// Schedule initial hide on mount.
scheduleHide();

onBeforeUnmount(() => {
  if (hideTimer != null) clearTimeout(hideTimer);
});
</script>

<style scoped>
.player-controls {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  pointer-events: none;
  z-index: 10;
  transition: opacity 0.25s ease;
}

.player-controls.controls-hidden {
  opacity: 0;
}

.player-controls.controls-visible {
  opacity: 1;
}

/* When visible, the controls receive pointer events; when hidden, they don't (so the
   video area is click-through for the center play button). */
.player-controls.controls-visible {
  pointer-events: auto;
}

.controls-top-bar,
.controls-bottom-bar {
  display: flex;
  width: 100%;
  box-sizing: border-box;
  padding: 12px 16px;
  background: linear-gradient(to bottom, rgba(0, 0, 0, 0.7), rgba(0, 0, 0, 0));
  pointer-events: auto;
}

.controls-bottom-bar {
  flex-direction: column;
  gap: 8px;
  background: linear-gradient(to top, rgba(0, 0, 0, 0.85), rgba(0, 0, 0, 0));
}

.controls-top-bar {
  justify-content: space-between;
  align-items: center;
}

.top-left,
.top-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.chapter-label {
  color: #f0f0ff;
  font-size: 13px;
  font-weight: 600;
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hwdec-badge {
  color: #aaffaa;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(40, 80, 40, 0.6);
  border: 1px solid rgba(80, 160, 80, 0.4);
}

.center-play-btn {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 72px;
  height: 72px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 32px;
  color: #fff;
  background: rgba(0, 0, 0, 0.6);
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-radius: 50%;
  cursor: pointer;
  transition: background 0.15s, transform 0.15s;
  pointer-events: auto;
}

.center-play-btn:hover {
  background: rgba(0, 0, 0, 0.8);
  transform: translate(-50%, -50%) scale(1.05);
}

.scrubber-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.time-display {
  color: #f0f0ff;
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  min-width: 56px;
  text-align: center;
  white-space: nowrap;
}

.time-separator {
  min-width: 100px;
  color: #ccccee;
}

.scrubber {
  flex: 1;
  height: 6px;
  -webkit-appearance: none;
  appearance: none;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 3px;
  cursor: pointer;
  outline: none;
}

.scrubber::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 14px;
  height: 14px;
  background: #6c63ff;
  border-radius: 50%;
  cursor: pointer;
  border: 2px solid #fff;
}

.scrubber::-moz-range-thumb {
  width: 14px;
  height: 14px;
  background: #6c63ff;
  border-radius: 50%;
  cursor: pointer;
  border: 2px solid #fff;
}

.scrubber:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.buttons-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.buttons-left,
.buttons-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  height: 36px;
  padding: 0 8px;
  color: #f0f0ff;
  background: transparent;
  border: 0;
  border-radius: 6px;
  font-size: 16px;
  font-weight: 700;
  cursor: pointer;
  transition: background 0.15s;
}

.icon-btn:hover {
  background: rgba(255, 255, 255, 0.15);
}

.icon-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.volume-group {
  display: flex;
  align-items: center;
  gap: 4px;
}

.volume-slider {
  width: 80px;
  height: 4px;
  -webkit-appearance: none;
  appearance: none;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
  cursor: pointer;
  outline: none;
}

.volume-slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 12px;
  height: 12px;
  background: #fff;
  border-radius: 50%;
  cursor: pointer;
}

.volume-slider::-moz-range-thumb {
  width: 12px;
  height: 12px;
  background: #fff;
  border-radius: 50%;
  cursor: pointer;
  border: 0;
}

.menu-group {
  position: relative;
}

.menu-panel {
  position: absolute;
  bottom: 44px;
  right: 0;
  min-width: 220px;
  max-height: 320px;
  overflow-y: auto;
  padding: 8px;
  background: rgba(20, 20, 32, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(12px);
}

.menu-header {
  padding: 6px 8px;
  color: #8888aa;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  margin-bottom: 4px;
}

.menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  color: #f0f0ff;
  background: transparent;
  border: 0;
  border-radius: 4px;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition: background 0.1s;
}

.menu-item:hover {
  background: rgba(255, 255, 255, 0.1);
}

.menu-item.active {
  background: rgba(108, 99, 255, 0.25);
  color: #fff;
  font-weight: 700;
}

.track-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.track-lang {
  color: #8888aa;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
}

.settings-panel {
  min-width: 320px;
  max-height: 480px;
}

.settings-row {
  display: grid;
  grid-template-columns: 90px 1fr 50px;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  font-size: 12px;
  color: #f0f0ff;
}

.settings-checkbox-row {
  grid-template-columns: auto 1fr;
}

.settings-label {
  color: #ccccee;
  font-weight: 600;
}

.settings-value {
  color: #8888aa;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.settings-row input[type='range'] {
  height: 4px;
  -webkit-appearance: none;
  appearance: none;
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
  cursor: pointer;
  outline: none;
}

.settings-row input[type='range']::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 12px;
  height: 12px;
  background: #6c63ff;
  border-radius: 50%;
  cursor: pointer;
}

.settings-row input[type='range']::-moz-range-thumb {
  width: 12px;
  height: 12px;
  background: #6c63ff;
  border-radius: 50%;
  cursor: pointer;
  border: 0;
}

.settings-divider {
  height: 1px;
  background: rgba(255, 255, 255, 0.1);
  margin: 6px 0;
}

.reset-btn {
  justify-content: center;
  color: #ffaaaa;
}

/* Custom scrollbar for menu panels */
.menu-panel::-webkit-scrollbar {
  width: 6px;
}

.menu-panel::-webkit-scrollbar-track {
  background: transparent;
}

.menu-panel::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 3px;
}

.menu-panel::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}

@media (max-width: 640px) {
  .controls-top-bar,
  .controls-bottom-bar {
    padding: 8px 10px;
  }

  .time-separator {
    display: none;
  }

  .volume-slider {
    width: 50px;
  }

  .settings-panel {
    min-width: 280px;
  }

  .center-play-btn {
    width: 56px;
    height: 56px;
    font-size: 24px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .player-controls {
    transition: none;
  }

  .center-play-btn {
    transition: none;
  }
}
</style>
