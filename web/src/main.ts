import { createApp } from 'vue';
import { createPinia } from 'pinia';
import router from './router';
import App from './App.vue';
import './style.css';
import { initTauriListeners } from './lib/runtime/tauri';

const app = createApp(App);

app.use(createPinia());
app.use(router);

// Mount the app first
app.mount('#app');

// Initialize Tauri event listeners after app is mounted
// This ensures Tauri internals are ready
initTauriListeners().catch(console.error);
