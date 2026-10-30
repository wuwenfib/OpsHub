import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 后端地址，默认本机 8080；后端在别处时：OPSHUB_BACKEND=http://x.x.x.x:8080 npm run dev
const backend = process.env.OPSHUB_BACKEND || 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    strictPort: true,
    // 开发代理：浏览器只和 5173 打交道，/api 和 /healthz 由 Vite 转发给后端。
    // 浏览器看来是同源请求，所以后端不需要处理 CORS。
    proxy: {
      '/api': backend,
      '/healthz': backend,
    },
  },
})
