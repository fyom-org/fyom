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

      // 生产环境启用压缩
      isProd &&
        compression({
          algorithm: 'gzip',
          ext: '.gz',
          threshold: 1024,
          deleteOriginFile: false,
        }),

      // 生产环境启用 bundle 分析
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
      // 生产环境不生成 source map
      sourcemap: false,

      // 目标浏览器
      target: 'esnext',

      // 使用 esbuild 压缩
      minify: 'esbuild',

      // 移除调试相关
      esbuild: {
        drop: ['console', 'debugger'],
        legalComments: 'none',
      },

      // 报告压缩大小
      reportCompressedSize: true,

      // 禁用 module preload polyfill
      modulePreload: {
        polyfill: false,
      },

      // CSS 代码分割
      cssCodeSplit: true,

      // 内联资源大小限制
      assetsInlineLimit: 4096,

      // Rollup 配置
      rollupOptions: {
        output: {
          // 手动分割依赖
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

            return 'vendor';
          },
        },
      },
    },
  };
});
