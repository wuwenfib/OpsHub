<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { levelTag } from '../format'

const router = useRouter()
const systems = ref([])
const unregistered = ref([]) // 工单里出现过、但台账里没有的 system 标签
const loading = ref(true)
const error = ref('')

const keyword = ref('')
const level = ref(0) // 0 = 全部

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return systems.value.filter(
    (s) =>
      (level.value === 0 || s.level === level.value) &&
      (!kw || s.code.toLowerCase().includes(kw) || s.name.toLowerCase().includes(kw)),
  )
})

onMounted(async () => {
  try {
    const [sys, tickets] = await Promise.all([api.get('/api/systems'), api.get('/api/tickets')])
    systems.value = sys
    // system_code 为空说明工单关联不上 CMDB：台账缺了这个系统
    unregistered.value = [...new Set(tickets.filter((t) => !t.system_code).map((t) => t.system_label || '（无标签）'))]
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
})

const open = (row) => router.push(`/systems/${row.code}`)
</script>

<template>
  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />
  <template v-else>
    <el-alert
      v-if="unregistered.length"
      type="warning"
      show-icon
      :closable="false"
      class="block"
      :title="`有 ${unregistered.length} 个告警来源未在台账登记：${unregistered.join('、')}`"
      description="这些告警生成的工单无法确定责任厂商，只能按二级系统计算 SLA。请补录台账。"
    />

    <div class="toolbar">
      <el-input v-model="keyword" placeholder="搜索编码或名称" clearable style="width: 240px" />
      <el-radio-group v-model="level">
        <el-radio-button :value="0">全部</el-radio-button>
        <el-radio-button :value="1">一级</el-radio-button>
        <el-radio-button :value="2">二级</el-radio-button>
        <el-radio-button :value="3">三级</el-radio-button>
      </el-radio-group>
      <span class="count">共 {{ filtered.length }} 个系统</span>
    </div>

    <el-table v-loading="loading" :data="filtered" stripe class="clickable" @row-click="open">
      <el-table-column prop="code" label="编码" width="140" />
      <el-table-column prop="name" label="名称" min-width="180" />
      <el-table-column prop="department" label="所属部门" min-width="150" />
      <el-table-column label="等级" width="80">
        <template #default="{ row }">
          <el-tag :type="levelTag(row.level).type" effect="dark" size="small">{{ levelTag(row.level).text }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="厂商" min-width="120">
        <template #default="{ row }">{{ row.vendor_name || '-' }}</template>
      </el-table-column>
      <el-table-column prop="owner" label="负责人" width="100" />
    </el-table>
  </template>
</template>

<style scoped>
.block {
  margin-bottom: 16px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
}
.count {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.clickable :deep(.el-table__row) {
  cursor: pointer;
}
</style>
