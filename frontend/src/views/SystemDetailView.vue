<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { contractTag, formatTime, levelTag, statusTag } from '../format'

const route = useRoute()
const router = useRouter()

const sys = ref(null)
const openTickets = ref([])
const loading = ref(true)
const error = ref('')

const contract = computed(() => contractTag(sys.value?.vendor?.contract_end))

async function load(code) {
  loading.value = true
  error.value = ''
  try {
    const [detail, tickets] = await Promise.all([
      api.get(`/api/systems/${encodeURIComponent(code)}`),
      api.get(`/api/tickets?system=${encodeURIComponent(code)}`),
    ])
    sys.value = detail
    openTickets.value = tickets.filter((t) => t.status !== 'closed')
  } catch (e) {
    sys.value = null
    error.value = e.message
  } finally {
    loading.value = false
  }
}

// 从一个系统点依赖跳到另一个系统时，路由复用同一个组件实例（不会重新 mounted），
// 所以监听路由参数变化来重新加载。immediate: 首次进入也执行一次。
watch(() => route.params.code, (code) => code && load(code), { immediate: true })

const goSystem = (code) => router.push(`/systems/${code}`)
</script>

<template>
  <el-page-header class="block" @back="router.push('/systems')">
    <template #content>
      <span v-if="sys">
        {{ sys.code }} {{ sys.name }}
        <el-tag :type="levelTag(sys.level).type" effect="dark" size="small">{{ levelTag(sys.level).text }}系统</el-tag>
      </span>
      <span v-else>{{ route.params.code }}</span>
    </template>
  </el-page-header>

  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />

  <div v-else v-loading="loading">
    <template v-if="sys">
      <el-row :gutter="16">
        <el-col :span="12">
          <el-card shadow="never" header="基本信息" class="block">
            <el-descriptions :column="1" border>
              <el-descriptions-item label="编码">{{ sys.code }}</el-descriptions-item>
              <el-descriptions-item label="名称">{{ sys.name }}</el-descriptions-item>
              <el-descriptions-item label="所属部门">{{ sys.department || '-' }}</el-descriptions-item>
              <el-descriptions-item label="负责人">{{ sys.owner || '-' }}</el-descriptions-item>
              <el-descriptions-item label="说明">{{ sys.description || '-' }}</el-descriptions-item>
            </el-descriptions>
          </el-card>
        </el-col>

        <el-col :span="12">
          <el-card shadow="never" header="运维厂商" class="block">
            <el-descriptions v-if="sys.vendor" :column="1" border>
              <el-descriptions-item label="厂商">{{ sys.vendor.name }}</el-descriptions-item>
              <el-descriptions-item label="联系人">{{ sys.vendor.contact || '-' }}</el-descriptions-item>
              <el-descriptions-item label="电话">{{ sys.vendor.phone || '-' }}</el-descriptions-item>
              <el-descriptions-item label="合同到期">
                {{ sys.vendor.contract_end || '-' }}
                <el-tag v-if="contract" :type="contract.type" size="small" class="ml">{{ contract.text }}</el-tag>
              </el-descriptions-item>
            </el-descriptions>
            <el-empty v-else description="未关联厂商：出了故障找不到责任方" :image-size="60" />
          </el-card>
        </el-col>
      </el-row>

      <el-card shadow="never" class="block">
        <template #header>依赖关系 <span class="hint">点击可跳转</span></template>
        <div class="dep-row">
          <span class="dep-label">依赖（上游）：</span>
          <el-tag v-for="d in sys.depends_on" :key="d.code" class="dep" @click="goSystem(d.code)">
            {{ d.code }} {{ d.name }}
          </el-tag>
          <span v-if="!sys.depends_on.length" class="none">无</span>
        </div>
        <div class="dep-row">
          <span class="dep-label">被依赖（下游）：</span>
          <el-tag v-for="d in sys.depended_by" :key="d.code" type="warning" class="dep" @click="goSystem(d.code)">
            {{ d.code }} {{ d.name }}
          </el-tag>
          <span v-if="!sys.depended_by.length" class="none">无</span>
        </div>
        <div v-if="sys.depended_by.length" class="hint">
          本系统故障时，以上 {{ sys.depended_by.length }} 个下游系统可能同时受影响。
        </div>
      </el-card>

      <el-card shadow="never" :header="`部署主机（${sys.hosts.length}）`" class="block">
        <el-table :data="sys.hosts" size="small">
          <el-table-column prop="hostname" label="主机名" />
          <el-table-column prop="ip" label="IP" />
          <el-table-column prop="os" label="操作系统" />
          <el-table-column prop="env" label="环境" width="80" />
        </el-table>
      </el-card>

      <el-card shadow="never" :header="`在办工单（${openTickets.length}）`" class="block">
        <el-table :data="openTickets" size="small" empty-text="没有未关闭的工单">
          <el-table-column prop="id" label="#" width="60" />
          <el-table-column label="级别" width="70">
            <template #default="{ row }">{{ row.severity }}</template>
          </el-table-column>
          <el-table-column prop="title" label="标题" min-width="220" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).text }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="SLA" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.sla.overdue" type="danger" size="small">超时</el-tag>
              <span v-else class="none">正常</span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>
  </div>
</template>

<style scoped>
.block {
  margin-bottom: 16px;
}
.ml {
  margin-left: 8px;
}
.dep-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}
.dep-label {
  width: 120px;
  white-space: nowrap;
  color: var(--el-text-color-secondary);
}
.dep {
  cursor: pointer;
}
.none,
.hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.hint {
  margin-left: 8px;
}
</style>
