import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import router from './router';
import './style.css';
import { initDesktopListeners, isDesktopEnvironment } from './lib/runtime/desktop';
import { useUserStore } from '@/stores/user';
import { useSystemStore } from '@/stores/system';
import i18n from '@/plugins/i18n';
import { applyInitialLocale } from '@/composables/useLocale';

const app = createApp(App);

const pinia = createPinia();
app.use(pinia);

// Install vue-i18n before any store or component reads locale state.
// `applyInitialLocale()` runs AFTER pinia is installed so Phase 1 can later
// consult the user / system stores during resolution. In Phase 0 it only
// reads `navigator.language`.
app.use(i18n);
applyInitialLocale();

const systemStore = useSystemStore();
const userStore = useUserStore();

/**
 * Bootstrap application state before installing the router.
 *
 * Why:
 * - Prevent router initial navigation from racing system/auth restoration
 * - Avoid hard-refresh deep-link issues on protected/admin routes
 * - Ensure stores are in a stable state before first route resolution
 */
async function bootstrapApplicationState(): Promise<void> {
  // 1. Resolve system state first.
  try {
    await systemStore.fetchSystemStatus();
  } catch (err) {
    console.error('[main] fetchSystemStatus failed:', err);
  }

  // 2. In desktop mode, try backend bootstrap auth only when there is no local token.
  try {
    if (isDesktopEnvironment() && !localStorage.getItem('token')) {
      await userStore.bootstrapDesktopAuth();
    }
  } catch (err) {
    console.error('[main] bootstrapDesktopAuth failed:', err);
  }

  // 3. Resolve auth state explicitly before router installation.
  //
  // If a token exists, rehydrate the session so user + role are available
  // before the first protected/admin navigation runs.
  //
  // If no token exists and auth is still unknown, force anonymous state so
  // the guard does not have to reason about an indeterminate session.
  try {
    if (localStorage.getItem('token')) {
      await userStore.rehydrateSession();
    } else if (userStore.status === 'unknown') {
      userStore.setAnonymous();
    }
  } catch (err) {
    console.error('[main] rehydrateSession failed:', err);

    // Fail safe: if rehydration blows up and auth is still unresolved,
    // do not leave the app in an indeterminate state without a persisted token.
    if (!localStorage.getItem('token') && userStore.status === 'unknown') {
      userStore.setAnonymous();
    }
  }
}

await bootstrapApplicationState();

// Install router only after bootstrap completes.
// This is critical for hard-refresh on protected/admin deep links.
app.use(router);

// Wait for router initial navigation to settle before mounting.
await router.isReady();

// Mount the app after stores + router are ready.
app.mount('#app');

// Initialize desktop listeners after mount.
// This ensures the Vue app and runtime environment are both ready.
initDesktopListeners().catch((err) => {
  console.error('[main] initDesktopListeners failed:', err);
});
