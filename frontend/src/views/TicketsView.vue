<script setup>
import { onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { api } from '../api'
import { formatTime, severityType, statusTag } from '../format'
import { loadOperator, saveOperator } from '../operator'

const router = useRouter()

// ---------- 列表与筛选（筛选交给后端：?status=&system=&overdue=true） ----------
const tickets = ref([])
const systems = ref([])
const loading = ref(false)
const error = ref('')
const filter = reactive({ status: '', system: '', overdue: false })
const statusOptions = ['new', 'acknowledged', 'resolved', 'reopened', 'closed']

async function load() {
  loading.value = true
  try {
    const q = new URLSearchParams()
    if (filter.status) q.set('status', filter.status)
    if (filter.system) q.set('system', filter.system)
    if (filter.overdue) q.set('overdue', 'true')
    tickets.value = await api.get(`/api/tickets?${q}`)
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch(filter, load) // 任一筛选条件变化就重新查询

// 值班场景：每 30 秒自动刷新，新告警单会自己冒出来
let timer
onMounted(async () => {
  load()
  timer = setInterval(load, 30000)
  try {
    systems.value = await api.get('/api/systems')
  } catch {
    // 系统下拉框加载失败不影响列表
  }
})
onUnmounted(() => clearInterval(timer))

const rowClass = ({ row }) => (row.sla.overdue ? 'overdue-row' : '')
const open = (row) => router.push(`/tickets/${row.id}`)

// ---------- 手工建单 ----------
const dialog = ref(false)
const saving = ref(false)
const form = reactive({ title: '', system_code: '', severity: 'P3', type: 'incident', note: '', operator: '' })

function openCreate() {
  Object.assign(form, { title: '', system_code: '', severity: 'P3', type: 'incident', note: '', operator: loadOperator() })
  dialog.value = true
}

async function submitCreate() {
  if (!form.title.trim() || !form.system_code || !form.operator.trim()) {
    ElMessage.warning('标题、系统、操作人都必须填写')
    return
  }
  saving.value = true
  try {
    const t = await api.post('/api/tickets', { ...form })
    saveOperator(form.operator.trim())
    dialog.value = false
    ElMessage.success(`已创建工单 #${t.id}`)
    router.push(`/tickets/${t.id}`)
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="toolbar">
    <el-select v-model="filter.status" placeholder="全部状态" clearable style="width: 130px">
      <el-option v-for="s in statusOptions" :key="s" :label="statusTag(s).text" :value="s" />
    </el-select>
    <el-select v-model="filter.system" placeholder="全部系统" clearable filterable style="width: 220px">
      <el-option v-for="s in systems" :key="s.code" :label="`${s.code} ${s.name}`" :value="s.code" />
    </el-select>
    <el-switch v-model="filter.overdue" active-text="只看超时" />
    <span class="count">共 {{ tickets.length }} 张</span>
    <div class="spacer" />
    <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
    <el-button type="primary" :icon="Plus" @click="openCreate">手工建单</el-button>
  </div>

  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" class="block" />

  <el-table
    :data="tickets"
    :row-class-name="rowClass"
    class="clickable"
    empty-text="没有符合条件的工单"
    @row-click="open"
  >
    <el-table-column prop="id" label="#" width="60" />
    <el-table-column label="级别" width="70">
      <template #default="{ row }">
        <el-tag :type="severityType(row.severity)" effect="dark" size="small">{{ row.severity }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column prop="title" label="标题" min-width="240" show-overflow-tooltip />
    <el-table-column label="系统" min-width="170">
      <template #default="{ row }">
        <span v-if="row.system_code">{{ row.system_code }} {{ row.system_name }}</span>
        <span v-else><el-tag type="warning" size="small">未登记</el-tag> {{ row.system_label }}</span>
      </template>
    </el-table-column>
    <el-table-column label="厂商" width="100">
      <template #default="{ row }">{{ row.vendor_name || '-' }}</template>
    </el-table-column>
    <el-table-column label="状态" width="90">
      <template #default="{ row }">
        <el-tag :type="statusTag(row.status).type" size="small">{{ statusTag(row.status).text }}</el-tag>
      </template>
    </el-table-column>
    <el-table-column label="处理人" width="110">
      <template #default="{ row }">{{ row.assignee || '-' }}</template>
    </el-table-column>
    <el-table-column label="创建时间" width="170">
      <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
    </el-table-column>
    <el-table-column label="SLA" width="110">
      <template #default="{ row }">
        <el-tag v-if="row.sla.response_overdue" type="danger" size="small">响应超时</el-tag>
        <el-tag v-else-if="row.sla.resolve_overdue" type="danger" size="small">恢复超时</el-tag>
        <span v-else class="muted">正常</span>
      </template>
    </el-table-column>
    <el-table-column label="来源" width="70">
      <template #default="{ row }">{{ row.source === 'alert' ? '告警' : '手工' }}</template>
    </el-table-column>
  </el-table>

  <el-dialog v-model="dialog" title="手工建单" width="520px">
    <el-form label-width="80px">
      <el-form-item label="标题" required>
        <el-input v-model="form.title" placeholder="例如：群众反映办件提交失败" />
      </el-form-item>
      <el-form-item label="系统" required>
        <el-select v-model="form.system_code" filterable placeholder="选择已登记的系统" style="width: 100%">
          <el-option v-for="s in systems" :key="s.code" :label="`${s.code} ${s.name}`" :value="s.code" />
        </el-select>
      </el-form-item>
      <el-form-item label="级别">
        <el-radio-group v-model="form.severity">
          <el-radio-button v-for="p in ['P1', 'P2', 'P3', 'P4']" :key="p" :value="p">{{ p }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="类型">
        <el-radio-group v-model="form.type">
          <el-radio value="incident">事件</el-radio>
          <el-radio value="problem">问题</el-radio>
          <el-radio value="change">变更</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="说明">
        <el-input v-model="form.note" type="textarea" :rows="2" placeholder="来源、现象等" />
      </el-form-item>
      <el-form-item label="操作人" required>
        <el-input v-model="form.operator" placeholder="你的姓名" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="dialog = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submitCreate">创建</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}
.spacer {
  flex: 1;
}
.count,
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.block {
  margin-bottom: 16px;
}
.clickable :deep(.el-table__row) {
  cursor: pointer;
}
/* 超时工单整行标红 */
.clickable :deep(.overdue-row) {
  --el-table-tr-bg-color: var(--el-color-danger-light-9);
}
</style>
