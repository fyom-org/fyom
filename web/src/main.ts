import { createApp } from 'vue';
import { createPinia } from 'pinia';
import router from './router';
import App from './App.vue';
import './style.css';
import { initTauriListeners } from './lib/runtime/tauri';

const app = createApp(App);

// Initialize Tauri event listeners for sidecar communication
initTauriListeners().catch(console.error);

app.use(createPinia());
app.use(router);
app.mount('#app');
