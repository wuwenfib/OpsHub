import { createRouter, createWebHistory } from 'vue-router'

// 路由表：URL → 主区域显示哪个页面组件。meta.title 显示在顶栏。
// 页面组件用 () => import(...) 懒加载：访问到才下载对应代码。
const routes = [
  { path: '/', redirect: '/systems' },
  { path: '/systems', component: () => import('./views/SystemsView.vue'), meta: { title: '系统台账' } },
  { path: '/systems/:code', component: () => import('./views/SystemDetailView.vue'), meta: { title: '系统详情' } },
  { path: '/tickets', component: () => import('./views/TicketsView.vue'), meta: { title: '工单' } },
  { path: '/sla', component: () => import('./views/SlaView.vue'), meta: { title: 'SLA 看板' } },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})
