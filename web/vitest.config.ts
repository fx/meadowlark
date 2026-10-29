import { resolve } from 'node:path'
import preact from '@preact/preset-vite'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [preact()],
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    exclude: ['e2e/**', 'node_modules/**'],
    setupFiles: ['./src/test-setup.ts'],
    server: {
      deps: {
        // wouter imports react; inlining routes it through the preact/compat alias.
        inline: [/@radix-ui/, /@floating-ui/, /react-remove-scroll/, /wouter/],
      },
    },
    alias: {
      '@phosphor-icons/react': resolve(__dirname, './src/test-phosphor-mock.tsx'),
      'react-remove-scroll': resolve(__dirname, './src/test-remove-scroll-mock.tsx'),
      '@radix-ui/react-presence': resolve(__dirname, './src/test-presence-mock.tsx'),
      '@radix-ui/react-focus-scope': resolve(__dirname, './src/test-focus-scope-mock.tsx'),
      '@floating-ui/react-dom': resolve(__dirname, './src/test-floating-ui-mock.tsx'),
      // wouter's shim is CommonJS and would load real React; preact/compat has the hook.
      'use-sync-external-store/shim/index.js': 'preact/compat',
    },
    coverage: {
      provider: 'v8',
      exclude: ['src/test-*.tsx', 'src/test-*.ts', 'src/main.tsx'],
      thresholds: {
        lines: 100,
        functions: 100,
        branches: 100,
        statements: 100,
      },
    },
  },
})
