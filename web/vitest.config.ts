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
    setupFiles: ['./src/test-setup.ts'],
    server: {
      deps: {
        inline: [/@radix-ui/, /@floating-ui/, /react-remove-scroll/],
      },
    },
    alias: {
      '@phosphor-icons/react': resolve(__dirname, './src/test-phosphor-mock.tsx'),
      'react-remove-scroll': resolve(__dirname, './src/test-remove-scroll-mock.tsx'),
      '@radix-ui/react-presence': resolve(__dirname, './src/test-presence-mock.tsx'),
      '@radix-ui/react-focus-scope': resolve(__dirname, './src/test-focus-scope-mock.tsx'),
      '@floating-ui/react-dom': resolve(__dirname, './src/test-floating-ui-mock.tsx'),
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
