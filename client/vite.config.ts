import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: "/chores/",
  server: {
    // Mirror nginx's rewrite rules: strip /chores prefix before forwarding.
    proxy: {
      '/chores/api': {
        target: 'http://localhost:8080',
        rewrite: (path) => path.replace(/^\/chores/, ''),
      },
      '/chores/oauth': {
        target: 'http://localhost:8080',
        rewrite: (path) => path.replace(/^\/chores/, ''),
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
    exclude: ['**/node_modules/**', '**/e2e/**'],
  },
})
