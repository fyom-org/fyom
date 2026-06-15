import { createApp } from 'vue';
import { createPinia } from 'pinia';
import router from './router';
import App from './App.vue';
import './style.css';
import { initTauriListeners } from './lib/runtime/tauri';
import { useUserStore } from './stores/user';

const app = createApp(App);

const pinia = createPinia();
app.use(pinia);
app.use(router);

// Bootstrap auth session before mount so the router guard and components
// see the correct state on first render.
const userStore = useUserStore();
userStore.rehydrateSession();

// Mount the app
app.mount('#app');

// Initialize Tauri event listeners after app is mounted
// This ensures Tauri internals are ready
initTauriListeners().catch(console.error);
