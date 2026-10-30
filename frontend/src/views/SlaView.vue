<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { formatMinutes } from '../format'

const report = ref(null)
const error = ref('')

onMounted(async () => {
  try {
    report.value = await api.get('/api/stats/sla?group_by=vendor')
  } catch (e) {
    error.value = e.message
  }
})
</script>

<template>
  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />
  <el-empty
    v-else-if="report"
    :description="`近 30 天 ${report.total.tickets} 次故障，MTTR ${formatMinutes(report.total.mttr_minutes)}；图表在 3.4 实现`"
  />
</template>
