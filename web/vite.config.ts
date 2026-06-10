import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'node:path';

export default defineConfig({
  plugins: [vue()],

  // Resolve path aliases (fix "@/..." imports)
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },

  // Development server (HMR + debugging)
  server: {
    host: true, // Allow LAN / custom domains (e.g. fyom.example.com)
    port: 5173,
    allowedHosts: true, // Allow custom domain access

    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,

        // Debug API proxy traffic
        configure: (proxy) => {
          proxy.on('proxyReq', (_, req) => {
            console.log('➡️ [API]', req.method, req.url);
          });
          proxy.on('proxyRes', (res, req) => {
            console.log('⬅️ [API]', res.statusCode, req.url);
          });
        },
      },
    },
  },

  // Preview server (serves built assets, no HMR)
  preview: {
    host: true,
    port: 4173,
    allowedHosts: true,
  },

  // Logging level
  logLevel: 'info',

  // Build config (debuggable preview)
  build: {
    sourcemap: true,
  },
});
