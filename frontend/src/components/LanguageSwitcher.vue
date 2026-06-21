<!--
  LanguageSwitcher — reusable locale selector (Phase 8 redesign).

  Design:
  - Custom dropdown (not a native <select>) with flag emoji + native name
    per locale, a checkmark on the active option, and smooth open/close
    animation. The native <select> couldn't show flags or per-option
    styling and rendered inconsistently across OSes.
  - Hot-swaps locale reactively via useLocale().setLocale() — NO page refresh.
    All t() consumers re-render immediately.
  - Accessible: full keyboard navigation (ArrowUp/Down, Enter, Esc, Tab),
    ARIA listbox semantics (role=listbox/option, aria-selected,
    aria-activedescendant, aria-expanded), and an aria-live announcement.
  - Phase 1 hook: when userStore.isAuthenticated is true, the switcher
    ALSO calls PUT /api/v1/auth/me/preferences to persist the choice
    server-side. Until Phase 1 ships, the choice is session-only.

  Usage:
    <LanguageSwitcher variant="compact" />
    <LanguageSwitcher variant="full" />
-->
<template>
  <div ref="rootEl" class="language-switcher" :class="`variant-${variant}`">
    <button
      ref="triggerRef"
      type="button"
      class="ls-trigger"
      :aria-label="$t('language.selectLabel')"
      :aria-haspopup="'listbox'"
      :aria-expanded="isOpen"
      :aria-activedescendant="isOpen ? activeOptionId : undefined"
      :disabled="persisting"
      @click="toggleOpen"
      @keydown="onTriggerKeydown"
    >
      <span class="ls-trigger-flag" aria-hidden="true">{{ localeFlag(currentLocale) }}</span>
      <span v-if="variant === 'full'" class="ls-trigger-name">{{ localeDisplayName(currentLocale) }}</span>
      <svg
        class="ls-chevron"
        :class="{ open: isOpen }"
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <polyline points="6 9 12 15 18 9" />
      </svg>
    </button>

    <transition name="ls-dropdown">
      <ul
        v-if="isOpen"
        ref="listRef"
        class="ls-dropdown"
        role="listbox"
        :aria-label="$t('language.label')"
        @keydown="onListKeydown"
      >
        <li
          v-for="(loc, idx) in supportedLocales"
          :id="optionId(loc)"
          :key="loc"
          role="option"
          :aria-selected="loc === currentLocale"
          class="ls-option"
          :class="{ active: loc === currentLocale, focused: idx === activeIndex }"
          tabindex="-1"
          @click="selectLocale(loc)"
          @mouseenter="activeIndex = idx"
          @mousemove="activeIndex = idx"
        >
          <span class="ls-option-flag" aria-hidden="true">{{ localeFlag(loc) }}</span>
          <span class="ls-option-name">{{ localeDisplayName(loc) }}</span>
          <svg
            v-if="loc === currentLocale"
            class="ls-check"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </li>
      </ul>
    </transition>

    <p v-if="announceText" class="ls-announce" role="status" aria-live="polite">
      {{ announceText }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, onMounted, onBeforeUnmount, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import { useLocale } from '@/composables/useLocale';
import { useUserStore } from '@/stores/user';
import type { LocaleCode } from '@/plugins/i18n';
import { localeDisplayName, localeFlag } from '@/plugins/i18n';

interface Props {
  /**
   * `compact` — name hidden, flag + chevron only (good for headers / tight spaces).
   * `full`    — flag + name + chevron (good for Settings / Profile pages).
   */
  variant?: 'compact' | 'full';
}

withDefaults(defineProps<Props>(), {
  variant: 'full',
});

const { t } = useI18n();
const { currentLocale, supportedLocales, setLocale } = useLocale();
const userStore = useUserStore();

const rootEl = ref<HTMLElement | null>(null);
const triggerRef = ref<HTMLButtonElement | null>(null);
const listRef = ref<HTMLUListElement | null>(null);

const isOpen = ref(false);
const activeIndex = ref(0);
const persisting = ref(false);
const announceText = ref('');

// Unique ID prefix for option elements (so aria-activedescendant resolves
// correctly when multiple switchers are mounted simultaneously).
const uid = Math.random().toString(36).slice(2, 9);

function optionId(loc: LocaleCode): string {
  return `ls-${uid}-opt-${loc}`;
}

// Computed so the template can read it as a plain value. A `get` accessor
// at the top level of <script setup> is syntactically valid TS but confuses
// some bundler parsers; computed is the idiomatic Vue 3 equivalent.
const activeOptionId = computed<string | undefined>(() => {
  const loc = supportedLocales.value[activeIndex.value];
  return loc ? optionId(loc) : undefined;
});

// --- Open / close --------------------------------------------------------

function toggleOpen(): void {
  if (isOpen.value) {
    close();
  } else {
    open();
  }
}

function open(): void {
  // Pre-select the active locale's index so keyboard nav starts there.
  const currentIdx = supportedLocales.value.indexOf(currentLocale.value);
  activeIndex.value = currentIdx >= 0 ? currentIdx : 0;
  isOpen.value = true;
  // Focus the list once it's rendered so arrow-key navigation works
  // immediately (the list element owns the keydown handler).
  void nextTick(() => {
    listRef.value?.focus();
  });
}

function close(): void {
  if (!isOpen.value) return;
  isOpen.value = false;
  // Return focus to the trigger so the user can re-open with the keyboard.
  triggerRef.value?.focus();
}

// --- Selection -----------------------------------------------------------

async function selectLocale(loc: LocaleCode): Promise<void> {
  if (loc === currentLocale.value) {
    close();
    return;
  }

  // Authenticated users: persist to server first. If the API call fails, we
  // do NOT change the local locale — the user sees an error and can retry.
  if (userStore.isAuthenticated) {
    persisting.value = true;
    try {
      await userStore.updatePreferredLanguage(loc);
      // Success: updatePreferredLanguage already called setLocale internally.
    } catch (err) {
      console.error('[LanguageSwitcher] Failed to persist preferred language:', err);
    } finally {
      persisting.value = false;
    }
  } else {
    // Anonymous users: session-only, no persistence.
    setLocale(loc);
  }

  // Announce the change for screen readers.
  announceText.value = t('language.label') + ': ' + localeDisplayName(loc);
  window.setTimeout(() => {
    announceText.value = '';
  }, 2000);

  close();
}

// --- Keyboard navigation -------------------------------------------------

function onTriggerKeydown(e: KeyboardEvent): void {
  switch (e.key) {
    case 'ArrowDown':
    case 'Enter':
    case ' ':
    case 'Spacebar':
      e.preventDefault();
      open();
      break;
    case 'Escape':
      if (isOpen.value) {
        e.preventDefault();
        close();
      }
      break;
  }
}

function onListKeydown(e: KeyboardEvent): void {
  const count = supportedLocales.value.length;
  if (count === 0) return;

  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault();
      activeIndex.value = (activeIndex.value + 1) % count;
      scrollOptionIntoView(activeIndex.value);
      break;
    case 'ArrowUp':
      e.preventDefault();
      activeIndex.value = (activeIndex.value - 1 + count) % count;
      scrollOptionIntoView(activeIndex.value);
      break;
    case 'Home':
      e.preventDefault();
      activeIndex.value = 0;
      scrollOptionIntoView(0);
      break;
    case 'End':
      e.preventDefault();
      activeIndex.value = count - 1;
      scrollOptionIntoView(count - 1);
      break;
    case 'Enter':
    case ' ':
    case 'Spacebar':
      e.preventDefault();
      void selectLocale(supportedLocales.value[activeIndex.value]);
      break;
    case 'Escape':
      e.preventDefault();
      close();
      break;
    case 'Tab':
      // Tab closes the menu (default focus move proceeds to the next element).
      close();
      break;
  }
}

function scrollOptionIntoView(idx: number): void {
  void nextTick(() => {
    const opts = listRef.value?.querySelectorAll<HTMLElement>('.ls-option');
    opts?.[idx]?.scrollIntoView({ block: 'nearest' });
  });
}

// --- Click-outside -------------------------------------------------------

function onDocumentClick(e: MouseEvent): void {
  if (!isOpen.value) return;
  if (rootEl.value && !rootEl.value.contains(e.target as Node)) {
    close();
  }
}

function onDocumentKeydown(e: KeyboardEvent): void {
  if (isOpen.value && e.key === 'Escape') {
    close();
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true);
  document.addEventListener('keydown', onDocumentKeydown);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true);
  document.removeEventListener('keydown', onDocumentKeydown);
});

// If the locale changes externally (e.g. another switcher instance),
// update the active index so the checkmark tracks the real state.
watch(currentLocale, (next) => {
  const idx = supportedLocales.value.indexOf(next);
  if (idx >= 0) activeIndex.value = idx;
});
</script>

<style scoped>
.language-switcher {
  position: relative;
  display: inline-flex;
  align-items: center;
  font-size: 13px;
}

/* --- Trigger button ----------------------------------------------------- */

.ls-trigger {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 32px;
  padding: 4px 10px 4px 8px;
  color: inherit;
  background: rgb(255 255 255 / 5%);
  border: 1px solid rgb(255 255 255 / 12%);
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease,
    box-shadow 0.15s ease;
}

.ls-trigger:hover:not(:disabled) {
  border-color: rgb(255 255 255 / 22%);
  background: rgb(255 255 255 / 8%);
}

.ls-trigger:focus-visible {
  outline: 2px solid #6c63ff;
  outline-offset: 1px;
  border-color: #6c63ff;
}

.ls-trigger:disabled {
  cursor: wait;
  opacity: 0.6;
}

.ls-trigger-flag {
  font-size: 16px;
  line-height: 1;
  /* Prevent flag emoji from inheriting the button's font-weight jitter. */
  font-family: 'Apple Color Emoji', 'Segoe UI Emoji', 'Noto Color Emoji', sans-serif;
}

.variant-compact .ls-trigger-flag {
  font-size: 17px;
}

.ls-trigger-name {
  white-space: nowrap;
  letter-spacing: 0.01em;
}

.ls-chevron {
  flex-shrink: 0;
  opacity: 0.6;
  transition: transform 0.2s ease, opacity 0.15s ease;
}

.ls-chevron.open {
  transform: rotate(180deg);
  opacity: 0.9;
}

/* --- Dropdown panel ----------------------------------------------------- */

.ls-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  z-index: 50;
  min-width: 100%;
  width: max-content;
  max-height: 280px;
  overflow-y: auto;
  margin: 0;
  padding: 4px;
  list-style: none;
  background: #1c1c2e;
  border: 1px solid rgb(255 255 255 / 14%);
  border-radius: 10px;
  box-shadow:
    0 8px 24px rgb(0 0 0 / 40%),
    0 2px 6px rgb(0 0 0 / 20%);
  /* Custom scrollbar for the rare case the list overflows. */
  scrollbar-width: thin;
  scrollbar-color: rgb(255 255 255 / 20%) transparent;
}

.ls-dropdown::-webkit-scrollbar {
  width: 6px;
}

.ls-dropdown::-webkit-scrollbar-thumb {
  background: rgb(255 255 255 / 18%);
  border-radius: 3px;
}

.ls-dropdown:focus {
  outline: none;
}

/* --- Option row --------------------------------------------------------- */

.ls-option {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 10px;
  border-radius: 6px;
  color: #c8c8dc;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.12s ease, color 0.12s ease;
  /* Reserve space for the checkmark so layout doesn't shift when it appears. */
  position: relative;
}

.ls-option:hover,
.ls-option.focused {
  background: rgb(255 255 255 / 8%);
  color: #f0f0f8;
}

.ls-option.active {
  color: #fff;
  font-weight: 600;
}

.ls-option-flag {
  font-size: 17px;
  line-height: 1;
  font-family: 'Apple Color Emoji', 'Segoe UI Emoji', 'Noto Color Emoji', sans-serif;
}

.ls-option-name {
  flex: 1;
  white-space: nowrap;
}

.ls-check {
  flex-shrink: 0;
  color: #6c63ff;
}

/* --- Open/close transition --------------------------------------------- */

.ls-dropdown-enter-active,
.ls-dropdown-leave-active {
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
  transform-origin: top left;
}

.ls-dropdown-enter-from,
.ls-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}

/* --- Screen-reader-only announcement ----------------------------------- */

.ls-announce {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* --- Light-surface variant (ProfileView, SettingsView) ----------------- */

:global(.light-surface) .ls-trigger {
  color: #1a1a2e;
  background: #fff;
  border-color: rgb(0 0 0 / 12%);
}

:global(.light-surface) .ls-trigger:hover:not(:disabled) {
  border-color: rgb(0 0 0 / 22%);
  background: #f8f8fc;
}

:global(.light-surface) .ls-dropdown {
  background: #fff;
  border-color: rgb(0 0 0 / 10%);
  box-shadow:
    0 8px 24px rgb(0 0 0 / 12%),
    0 2px 6px rgb(0 0 0 / 8%);
}

:global(.light-surface) .ls-option {
  color: #3a3a4e;
}

:global(.light-surface) .ls-option:hover,
:global(.light-surface) .ls-option.focused {
  background: rgb(108 99 255 / 8%);
  color: #1a1a2e;
}

:global(.light-surface) .ls-option.active {
  color: #1a1a2e;
}

:global(.light-surface) .ls-dropdown::-webkit-scrollbar-thumb {
  background: rgb(0 0 0 / 15%);
}

/* --- Compact variant tweaks -------------------------------------------- */

.variant-compact .ls-trigger {
  gap: 4px;
  padding: 4px 8px 4px 6px;
}

@media (prefers-reduced-motion: reduce) {
  .ls-trigger,
  .ls-chevron,
  .ls-option,
  .ls-dropdown-enter-active,
  .ls-dropdown-leave-active {
    transition: none;
  }
}
</style>
