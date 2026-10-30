<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

const systems = ref([])
const error = ref('')

onMounted(async () => {
  try {
    systems.value = await api.get('/api/systems')
  } catch (e) {
    error.value = e.message
  }
})
</script>

<template>
  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />
  <el-empty v-else :description="`已登记 ${systems.length} 个业务系统；台账列表与详情在 3.2 实现`" />
</template>
