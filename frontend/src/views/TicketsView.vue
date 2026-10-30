<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'

const tickets = ref([])
const error = ref('')
const overdue = computed(() => tickets.value.filter((t) => t.sla.overdue).length)

onMounted(async () => {
  try {
    tickets.value = await api.get('/api/tickets')
  } catch (e) {
    error.value = e.message
  }
})
</script>

<template>
  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />
  <el-empty v-else :description="`共 ${tickets.length} 张工单，其中 ${overdue} 张超时；工单列表与操作在 3.3 实现`" />
</template>
