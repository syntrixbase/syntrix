import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    global: 'window', // RxDB needs global to be defined
  },
  optimizeDeps: {
    include: ['@syntrix/client', '@syntrix/client/dist/index.js'],
    force: true,
  },
  build: {
    commonjsOptions: {
      include: [/node_modules/, /@syntrix\/client/],
      transformMixedEsModules: true,
    },
  },
  server: {
    port: 5173,
    strictPort: true,
  },
})
