import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Share the repo-root .env; Vite only exposes VITE_* variables to the browser.
  envDir: '..',
  // Vercel's build variables, for Sentry's release and environment.
  define: {
    __APP_RELEASE__: JSON.stringify(process.env.VERCEL_GIT_COMMIT_SHA ?? ''),
    __APP_ENVIRONMENT__: JSON.stringify(process.env.VERCEL_ENV ?? ''),
  },
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  build: {
    chunkSizeWarningLimit: 700,
    rolldownOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('katex') || id.includes('rehype') || id.includes('remark') || id.includes('react-markdown') || id.includes('micromark') || id.includes('mdast') || id.includes('hast')) {
            return 'markdown'
          }
          if (id.includes('@supabase')) {
            return 'supabase'
          }
          if (id.includes('@sentry')) {
            return 'sentry'
          }
          if (id.includes('motion') || id.includes('framer-motion')) {
            return 'motion'
          }
          return undefined
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
