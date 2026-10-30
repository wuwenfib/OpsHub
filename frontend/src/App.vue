<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Collection, DataAnalysis, Tickets } from '@element-plus/icons-vue'

const route = useRoute()
const title = computed(() => route.meta.title ?? '')

// 后端健康状态：每 15 秒探一次 /healthz（经 Vite 代理转发到后端）。
const backendOK = ref(null) // null = 尚未探测
let timer
async function probe() {
  try {
    const resp = await fetch('/healthz')
    backendOK.value = resp.ok
  } catch {
    backendOK.value = false
  }
}
onMounted(() => {
  probe()
  timer = setInterval(probe, 15000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <el-container class="layout">
    <el-aside width="200px" class="aside">
      <div class="logo">OpsHub</div>
      <!-- router 模式：index 即跳转路径；default-active 让当前页高亮 -->
      <el-menu router :default-active="route.path" class="menu">
        <el-menu-item index="/systems">
          <el-icon><Collection /></el-icon><span>系统台账</span>
        </el-menu-item>
        <el-menu-item index="/tickets">
          <el-icon><Tickets /></el-icon><span>工单</span>
        </el-menu-item>
        <el-menu-item index="/sla">
          <el-icon><DataAnalysis /></el-icon><span>SLA 看板</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <span class="title">{{ title }}</span>
        <el-tag v-if="backendOK === null" type="info">后端检测中</el-tag>
        <el-tag v-else-if="backendOK" type="success">后端正常</el-tag>
        <el-tag v-else type="danger">后端不可用</el-tag>
      </el-header>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style>
html,
body,
#app {
  margin: 0;
  height: 100%;
}
</style>

<style scoped>
.layout {
  height: 100%;
}
.aside {
  border-right: 1px solid var(--el-border-color-light);
}
.logo {
  height: 60px;
  line-height: 60px;
  text-align: center;
  font-size: 20px;
  font-weight: 600;
  color: var(--el-color-primary);
  border-bottom: 1px solid var(--el-border-color-light);
}
.menu {
  border-right: none;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--el-border-color-light);
}
.title {
  font-size: 18px;
  font-weight: 500;
}
</style>
