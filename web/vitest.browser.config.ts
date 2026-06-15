import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],

  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },

  server: {
    host: '127.0.0.1',
  },

  test: {
    include: ['tests/**/*.{test,spec}.{ts,tsx,js,jsx}'],

    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/.git/**',
      '**/.direnv/**',
      '**/result/**',
      '**/result-*/*',
    ],

    browser: {
      enabled: true,
      provider: 'playwright',
      headless: true,

      instances: [
        {
          browser: 'chromium',
          launch: {
            args: [
              '--no-sandbox',
              '--disable-setuid-sandbox',
              '--disable-dev-shm-usage',
              '--disable-gpu',
              '--no-proxy-server',
            ],
          },
        },
      ],
    },
  },
});
