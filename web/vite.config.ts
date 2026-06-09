import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],

  // Development server (used for active development with HMR)
  server: {
    host: true, // Allow access via LAN / custom domains (e.g. fyom.example.com)
    port: 5173,

    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,

        // Debug API proxy traffic (useful for backend integration)
        configure: (proxy) => {
          proxy.on('proxyReq', (_, req) => {
            console.log('➡️ [API]', req.method, req.url)
          })
          proxy.on('proxyRes', (res, req) => {
            console.log('⬅️ [API]', res.statusCode, req.url)
          })
        },
      },
    },
  },

  // Preview server (serves built assets, no HMR)
  preview: {
    host: true,         // Allow external access during preview
    port: 4173,
    allowedHosts: true, // Allow custom domain (e.g. fyom.example.com)
  },

  // General logging level for dev server
  // Options: 'info' | 'warn' | 'error' | 'debug'
  logLevel: 'info',

  // Build configuration (used for production build / preview)
  build: {
    sourcemap: true, // Enable source maps for debugging in preview mode
  },
})
