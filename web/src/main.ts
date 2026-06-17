import { createApp } from 'vue';
import { createPinia } from 'pinia';
import router from './router';
import App from './App.vue';
import './style.css';
import { initTauriListeners, isTauriEnvironment } from './lib/runtime/tauri';
import { useUserStore } from '@/stores/user';
import { useSystemStore } from '@/stores/system';

const app = createApp(App);

const pinia = createPinia();
app.use(pinia);
app.use(router);

// Bootstrap system + auth state before mount so the router guard
// and components see the correct state on first render.
const systemStore = useSystemStore();
const userStore = useUserStore();
await systemStore.fetchSystemStatus();

// In Tauri desktop mode, try to bootstrap from internal session endpoint
// if no local token exists.
if (isTauriEnvironment() && !localStorage.getItem('token')) {
  await userStore.bootstrapDesktopAuth();
}

// Then rehydrate session (will use the bootstrap token if it was stored)
await userStore.rehydrateSession();

// Mount the app
app.mount('#app');

// Wait for router to be ready (initial navigation complete)
await router.isReady();

// Initialize Tauri event listeners after app is mounted
// This ensures Tauri internals are ready
initTauriListeners().catch(console.error);
