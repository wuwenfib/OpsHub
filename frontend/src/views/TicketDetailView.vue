<script setup>
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '../api'
import { actionText, allowedActions, deadlinePhase, formatTime, severityType, statusTag } from '../format'
import { loadOperator, saveOperator } from '../operator'

const route = useRoute()
const router = useRouter()

const t = ref(null)
const loading = ref(true)
const error = ref('')

async function load() {
  try {
    t.value = await api.get(`/api/tickets/${route.params.id}`)
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch(() => route.params.id, (id) => id && load(), { immediate: true })

// 倒计时每 30 秒刷新一次
const now = ref(new Date())
const timer = setInterval(() => (now.value = new Date()), 30000)
onUnmounted(() => clearInterval(timer))

const responsePhase = computed(() => t.value && deadlinePhase(t.value.response_deadline, t.value.responded_at, now.value))
const resolvePhase = computed(() => t.value && deadlinePhase(t.value.resolve_deadline, t.value.resolved_at, now.value))

// 当前状态下可执行的操作。只有事件单（incident）支持流转。
const actions = computed(() => (t.value?.type === 'incident' ? (allowedActions[t.value.status] ?? []) : []))
const buttonType = { ack: 'primary', resolve: 'success', close: 'info', reopen: 'danger' }
const actionHint = {
  ack: '受理后开始计入处理人，响应时间以此刻为准。',
  resolve: '业务已恢复。恢复时间以此刻为准，等待甲方确认关单。',
  close: '确认问题已解决，关单后同一告警再次触发会新建工单。',
  reopen: '确认没有修好。会清空恢复时间，最终以下一次恢复为准。',
}

// ---------- 操作对话框（所有操作共用） ----------
const dialog = reactive({ visible: false, action: '', operator: '', note: '', saving: false })

function openAction(action) {
  Object.assign(dialog, { visible: true, action, operator: loadOperator(), note: '', saving: false })
}

async function submitAction() {
  if (!dialog.operator.trim()) {
    ElMessage.warning('请填写操作人')
    return
  }
  dialog.saving = true
  try {
    // 成功时后端返回最新的工单详情（含流水），直接替换
    t.value = await api.post(`/api/tickets/${t.value.id}/${dialog.action}`, {
      operator: dialog.operator.trim(),
      note: dialog.note.trim(),
    })
    saveOperator(dialog.operator.trim())
    dialog.visible = false
    ElMessage.success(`已${actionText[dialog.action]}`)
  } catch (e) {
    if (e.status === 409) {
      // 状态已被别人改过（例如同时有人处理），刷新到最新状态并说明
      dialog.visible = false
      await load()
      ElMessage.warning(`操作未执行：工单当前状态为「${statusTag(t.value.status).text}」，可能已被他人处理，页面已刷新`)
    } else {
      ElMessage.error(e.message)
    }
  } finally {
    dialog.saving = false
  }
}

// ---------- 流水时间线的样式 ----------
const logColor = {
  created: 'var(--el-color-primary)',
  duplicate_alert: 'var(--el-color-info)',
  alert_resolved: 'var(--el-color-primary)',
  ack: 'var(--el-color-success)',
  resolve: 'var(--el-color-success)',
  close: 'var(--el-color-info)',
  reopen: 'var(--el-color-danger)',
}
</script>

<template>
  <el-page-header class="block" @back="router.push('/tickets')">
    <template #content>
      <span v-if="t" class="head">
        <el-tag :type="severityType(t.severity)" effect="dark">{{ t.severity }}</el-tag>
        #{{ t.id }} {{ t.title }}
        <el-tag :type="statusTag(t.status).type">{{ statusTag(t.status).text }}</el-tag>
        <el-tag v-if="t.sla.overdue" type="danger" effect="plain">SLA 超时</el-tag>
      </span>
      <span v-else>工单 #{{ route.params.id }}</span>
    </template>
    <template #extra>
      <template v-if="t">
        <el-button v-for="a in actions" :key="a" :type="buttonType[a]" @click="openAction(a)">
          {{ actionText[a] }}
        </el-button>
        <span v-if="t.type !== 'incident'" class="muted">{{ t.type }} 类型工单暂不支持流转</span>
        <span v-else-if="!actions.length" class="muted">工单已关闭</span>
      </template>
    </template>
  </el-page-header>

  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" />

  <el-row v-else-if="t" v-loading="loading" :gutter="16">
    <el-col :span="11">
      <el-card shadow="never" header="SLA 时限" class="block">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="响应截止">
            {{ formatTime(t.response_deadline) }}
            <el-tag :type="responsePhase.type" size="small" class="ml">{{ responsePhase.text }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="受理时间">{{ formatTime(t.responded_at) }}</el-descriptions-item>
          <el-descriptions-item label="恢复截止">
            {{ formatTime(t.resolve_deadline) }}
            <el-tag :type="resolvePhase.type" size="small" class="ml">{{ resolvePhase.text }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="恢复时间">{{ formatTime(t.resolved_at) }}</el-descriptions-item>
          <el-descriptions-item label="关闭时间">{{ formatTime(t.closed_at) }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card shadow="never" header="工单信息" class="block">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="系统">
            <el-link v-if="t.system_code" type="primary" @click="router.push(`/systems/${t.system_code}`)">
              {{ t.system_code }} {{ t.system_name }}
            </el-link>
            <span v-else><el-tag type="warning" size="small">未登记</el-tag> {{ t.system_label }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="厂商">{{ t.vendor_name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="处理人">{{ t.assignee || '-' }}</el-descriptions-item>
          <el-descriptions-item label="类型">{{ t.type }}</el-descriptions-item>
          <el-descriptions-item label="来源">{{ t.source === 'alert' ? '告警自动建单' : '手工建单' }}</el-descriptions-item>
          <el-descriptions-item v-if="t.alert_fingerprint" label="告警指纹">
            <code>{{ t.alert_fingerprint }}</code>
          </el-descriptions-item>
          <el-descriptions-item label="创建时间">{{ formatTime(t.created_at) }}</el-descriptions-item>
        </el-descriptions>
      </el-card>
    </el-col>

    <el-col :span="13">
      <el-card shadow="never" :header="`处理流水（${t.logs.length}）`">
        <el-timeline>
          <el-timeline-item
            v-for="l in t.logs"
            :key="l.id"
            :timestamp="formatTime(l.created_at)"
            :color="logColor[l.action]"
            placement="top"
          >
            <div class="log-head">
              <b>{{ actionText[l.action] ?? l.action }}</b>
              <span class="muted">{{ l.actor }}</span>
              <span v-if="l.to_status" class="muted">
                {{ l.from_status ? statusTag(l.from_status).text : '' }} → {{ statusTag(l.to_status).text }}
              </span>
            </div>
            <div v-if="l.note" class="log-note">{{ l.note }}</div>
          </el-timeline-item>
        </el-timeline>
      </el-card>
    </el-col>
  </el-row>

  <el-dialog v-model="dialog.visible" :title="`${actionText[dialog.action]} 工单 #${t?.id}`" width="460px">
    <p class="muted">{{ actionHint[dialog.action] }}</p>
    <el-form label-width="70px">
      <el-form-item label="操作人" required>
        <el-input v-model="dialog.operator" placeholder="你的姓名，例如：星河-张工" />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="dialog.note" type="textarea" :rows="3" placeholder="处理情况、原因等（会记入流水）" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="dialog.visible = false">取消</el-button>
      <el-button :type="buttonType[dialog.action]" :loading="dialog.saving" @click="submitAction">
        {{ actionText[dialog.action] }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.block {
  margin-bottom: 16px;
}
.head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.ml {
  margin-left: 8px;
}
:deep(.el-descriptions__label) {
  white-space: nowrap;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.log-head {
  display: flex;
  gap: 12px;
  align-items: baseline;
}
.log-note {
  margin-top: 4px;
  color: var(--el-text-color-regular);
  font-size: 13px;
}
</style>
