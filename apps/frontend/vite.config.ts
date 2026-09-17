import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const backendTarget = process.env.BACKEND_URL ?? 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    proxy: {
      '/api': backendTarget,
      '/ws': { target: backendTarget, ws: true },
    },
  },
})
