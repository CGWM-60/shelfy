import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import { configDefaults } from 'vitest/config'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080'

  return {
    plugins: [react()],
    server: {
      proxy: {
        '/api': {
          target: proxyTarget,
          changeOrigin: true
        },
        '/files': {
          target: proxyTarget,
          changeOrigin: true
        }
      }
    },
    test: {
      environment: 'jsdom',
      setupFiles: './src/test/setup.ts',
      globals: true,
      exclude: [...configDefaults.exclude, 'e2e/**']
    }
  }
})
