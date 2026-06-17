import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import path from 'node:path';
import compression from 'vite-plugin-compression';
import { visualizer } from 'rollup-plugin-visualizer';
import zlib from 'node:zlib';

export default defineConfig(({ mode }) => {
  const isProd = mode === 'production';

  return {
    plugins: [
      vue(),

      // Enable compression in production
      isProd &&
        compression({
          algorithm: 'gzip',
          ext: '.gz',
          threshold: 1024,
          deleteOriginFile: false,
        }),

      // Enable bundle analysis in production
      isProd &&
        visualizer({
          filename: 'dist/stats.html',
          gzipSize: true,
          brotliSize: true,
          open: false,
        }),
    ].filter(Boolean),

    // Resolve path aliases
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
      },
    },

    // Development server
    server: {
      host: true,
      port: 5173,
      allowedHosts: true,
      proxy: {
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
          secure: false,
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

    // Preview server
    preview: {
      host: true,
      port: 4173,
      allowedHosts: true,
    },

    // Logging level
    logLevel: 'info',

    // Build config
    build: {
      // Disable source maps in production
      sourcemap: false,

      // Target browsers
      target: 'esnext',

      // Use esbuild for minification
      minify: 'esbuild',

      // Strip debug primitives (console, debugger)
      esbuild: {
        drop: ['console', 'debugger'],
        legalComments: 'none',
      },

      // Report compressed size
      reportCompressedSize: true,

      // Disable module preload polyfill
      modulePreload: {
        polyfill: false,
      },

      // CSS code splitting
      cssCodeSplit: true,

      // Inline asset size limit
      assetsInlineLimit: 4096,

      // Rollup configuration
      rollupOptions: {
        output: {
          // Manually split dependencies
          manualChunks(id) {
            if (!id.includes('node_modules')) return;

            if (id.includes('/vue/') || id.includes('\\vue\\')) {
              return 'vue';
            }

            if (id.includes('vue-router')) {
              return 'router';
            }

            if (id.includes('pinia')) {
              return 'pinia';
            }

            if (id.includes('axios')) {
              return 'axios';
            }

            if (id.includes('vue-i18n') || id.includes('@intlify')) {
              return 'i18n';
            }

            return 'vendor';
          },
        },
      },
    },
  };
});
