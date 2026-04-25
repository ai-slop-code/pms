import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Backend target for the dev server's `/api` proxy. Defaults to the local
// dev backend on :8080; the e2e harness overrides this to point at the
// hermetic test backend on a different port.
const apiProxyTarget = process.env.VITE_DEV_API_PROXY ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: apiProxyTarget, changeOrigin: true },
    },
  },
})
