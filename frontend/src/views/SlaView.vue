<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { WarningFilled } from '@element-plus/icons-vue'
import { api } from '../api'
import { formatMinutes, formatPercent, formatTime } from '../format'
import EChart from '../components/EChart.vue'

const router = useRouter()

// ---------- 配色（dataviz 参考调色板，已用校验脚本验证：CVD ΔE 24.7，对比度 ≥ 3:1） ----------
const SERIES_1 = '#2a78d6' // 类别色 1：响应达标率 / MTTR
const SERIES_2 = '#eb6834' // 类别色 2：恢复达标率
const TEXT_REGULAR = '#606266' // 坐标轴文字用文字色，不用系列色
const TEXT_MUTED = '#909399'
const GRID_LINE = '#ebeef5'
const TARGET = 0.9 // 示例考核线：达标率 90%（以合同约定为准）

// ---------- 筛选：分组 + 日期范围（按北京时间自然日） ----------
const groupBy = ref('vendor')
const ymd = (d) => d.toLocaleDateString('sv-SE', { timeZone: 'Asia/Shanghai' }) // → "2026-10-30"
const daysAgo = (n) => new Date(Date.now() - n * 86400000)
const range = ref([ymd(daysAgo(29)), ymd(new Date())]) // 默认近 30 天（含今天）
const shortcuts = [
  { text: '近 7 天', value: () => [daysAgo(6), new Date()] },
  { text: '近 30 天', value: () => [daysAgo(29), new Date()] },
  {
    text: '本月',
    value: () => {
      const [y, m] = ymd(new Date()).split('-')
      return [new Date(`${y}-${m}-01T00:00:00+08:00`), new Date()]
    },
  },
]

const report = ref(null)
const loading = ref(false)
const error = ref('')

async function load() {
  if (!range.value) return
  loading.value = true
  try {
    // [开始日 00:00, 结束日次日 00:00)（北京时间），与后端左闭右开一致，结束日当天包含在内
    const from = new Date(`${range.value[0]}T00:00:00+08:00`)
    const to = new Date(new Date(`${range.value[1]}T00:00:00+08:00`).getTime() + 86400000)
    const q = new URLSearchParams({ group_by: groupBy.value, from: from.toISOString(), to: to.toISOString() })
    report.value = await api.get(`/api/stats/sla?${q}`)
    error.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch([groupBy, range], load, { immediate: true })

const groups = computed(() => report.value?.groups ?? [])
const total = computed(() => report.value?.total)
// 关联不上台账的分组：单独标出，提示台账需要补全
const isGap = (g) => g.name === '未登记系统' || g.key === '未关联厂商'
const label = (g) => (groupBy.value === 'system' && !isGap(g) ? g.key : g.name)

// ---------- 图 1：达标率对比（两个同单位的量 → 分组柱状图，一根 Y 轴） ----------
const pct = (r) => (r == null ? null : Math.round(r * 1000) / 10)
// 0% 的柱子高度为 0，看起来和"没有数据"一样，所以只给 0 值加上数值标签
const rateData = (rates) =>
  rates.map((r) => {
    const v = pct(r)
    return v === 0 ? { value: 0, label: { show: true, position: 'top', formatter: '0%', color: TEXT_REGULAR } } : v
  })
// 两个达标率都还没有结论（工单都没到期）的分组，在分组名下注明，避免把空白误读成 0
const rateAxisLabel = (g) =>
  g.response_rate == null && g.resolve_rate == null ? `${label(g)}\n（无结论）` : label(g)
const barStyle = (color) => ({
  type: 'bar',
  barMaxWidth: 24, // 细柱，不填满槽位
  barGap: '8%', // 相邻柱之间留约 2px 空隙，不描边
  itemStyle: { color, borderRadius: [4, 4, 0, 0] }, // 数据端 4px 圆角，基线端直角
  emphasis: { focus: 'none' },
})
const axisCommon = {
  axisLine: { lineStyle: { color: GRID_LINE } },
  axisTick: { show: false },
  // interval: 0 强制显示每个分组名（默认放不下时会隔一个隐藏，柱子就对不上名字了）；太长则换行
  axisLabel: { color: TEXT_REGULAR, interval: 0, width: 72, overflow: 'break' },
}

const rateOption = computed(() => ({
  grid: { left: 48, right: 72, top: 40, bottom: 32 },
  legend: { top: 0, left: 0, itemWidth: 12, itemHeight: 12, textStyle: { color: TEXT_REGULAR } },
  tooltip: {
    trigger: 'axis',
    axisPointer: { type: 'shadow' },
    formatter: (items) => {
      const g = groups.value[items[0].dataIndex]
      const line = (name, rate, met, tot) =>
        `${name}：${rate == null ? '无结论（尚未到期）' : `${formatPercent(rate)}（${met}/${tot}）`}`
      return [
        `<b>${label(g)}</b>`,
        line('响应达标率', g.response_rate, g.response_met, g.response_total),
        line('恢复达标率', g.resolve_rate, g.resolve_met, g.resolve_total),
      ].join('<br/>')
    },
  },
  xAxis: { type: 'category', data: groups.value.map(rateAxisLabel), ...axisCommon },
  yAxis: {
    type: 'value',
    min: 0,
    max: 100,
    axisLabel: { color: TEXT_MUTED, formatter: '{value}%' },
    splitLine: { lineStyle: { color: GRID_LINE } },
  },
  series: [
    {
      name: '响应达标率',
      data: rateData(groups.value.map((g) => g.response_rate)),
      ...barStyle(SERIES_1),
      markLine: {
        silent: true,
        symbol: 'none',
        lineStyle: { color: TEXT_MUTED, type: 'dashed', width: 1 },
        label: { formatter: `考核线 ${TARGET * 100}%`, color: TEXT_MUTED, position: 'end' },
        data: [{ yAxis: TARGET * 100 }],
      },
    },
    { name: '恢复达标率', data: rateData(groups.value.map((g) => g.resolve_rate)), ...barStyle(SERIES_2) },
  ],
}))

// ---------- 图 2：MTTR（单位是分钟，与百分比不同 → 单独一张图，不用双 Y 轴；单序列不需要图例） ----------
const mttrOption = computed(() => ({
  grid: { left: 64, right: 24, top: 40, bottom: 32 },
  tooltip: {
    trigger: 'axis',
    axisPointer: { type: 'shadow' },
    formatter: (items) => {
      const g = groups.value[items[0].dataIndex]
      const v = g.mttr_minutes == null ? '无（尚无已恢复工单）' : `${formatMinutes(g.mttr_minutes)}（已恢复 ${g.resolved} 单）`
      return `<b>${label(g)}</b><br/>MTTR：${v}`
    },
  },
  xAxis: { type: 'category', data: groups.value.map(label), ...axisCommon },
  yAxis: {
    type: 'value',
    name: '分钟',
    nameTextStyle: { color: TEXT_MUTED },
    axisLabel: { color: TEXT_MUTED },
    splitLine: { lineStyle: { color: GRID_LINE } },
  },
  series: [
    {
      name: 'MTTR',
      data: groups.value.map((g) => (g.mttr_minutes == null ? null : Math.round(g.mttr_minutes * 10) / 10)),
      ...barStyle(SERIES_1),
    },
  ],
}))

const ratio = (rate, met, tot) => (rate == null ? '-' : `${formatPercent(rate)}（${met}/${tot}）`)
</script>

<template>
  <div class="toolbar">
    <el-radio-group v-model="groupBy">
      <el-radio-button value="vendor">按厂商</el-radio-button>
      <el-radio-button value="system">按系统</el-radio-button>
    </el-radio-group>
    <el-date-picker
      v-model="range"
      type="daterange"
      value-format="YYYY-MM-DD"
      range-separator="至"
      :shortcuts="shortcuts"
      :clearable="false"
      style="width: 260px"
    />
    <span v-if="report" class="muted">统计时刻 {{ formatTime(report.generated_at) }}</span>
  </div>

  <el-alert v-if="error" type="error" :title="error" show-icon :closable="false" class="block" />

  <div v-else-if="report" v-loading="loading">
    <!-- 合计：单个醒目数字用指标卡，不画图 -->
    <div class="tiles">
      <div class="tile">
        <div class="tile-label">故障数</div>
        <div class="tile-value">{{ total.tickets }}</div>
        <div class="tile-sub">已恢复 {{ total.resolved }}</div>
      </div>
      <div class="tile">
        <div class="tile-label">MTTR</div>
        <div class="tile-value">{{ formatMinutes(total.mttr_minutes) }}</div>
        <div class="tile-sub">已恢复工单的平均值</div>
      </div>
      <div class="tile">
        <div class="tile-label">响应达标率</div>
        <div class="tile-value">{{ formatPercent(total.response_rate) }}</div>
        <div class="tile-sub">{{ total.response_met }}/{{ total.response_total }} 单（未到期的不计入）</div>
      </div>
      <div class="tile">
        <div class="tile-label">恢复达标率</div>
        <div class="tile-value">{{ formatPercent(total.resolve_rate) }}</div>
        <div class="tile-sub">{{ total.resolve_met }}/{{ total.resolve_total }} 单（未到期的不计入）</div>
      </div>
      <!-- 状态色必须配图标和文字，不能只靠颜色 -->
      <div
        class="tile clickable-tile"
        :class="{ critical: total.overdue_now > 0 }"
        @click="router.push('/tickets?overdue=true')"
      >
        <div class="tile-label">
          <el-icon v-if="total.overdue_now > 0"><WarningFilled /></el-icon>
          当前超时
        </div>
        <div class="tile-value">{{ total.overdue_now }}</div>
        <div class="tile-sub">{{ total.overdue_now > 0 ? '需要催办，点击查看' : '没有超时工单' }}</div>
      </div>
    </div>

    <el-empty v-if="!groups.length" description="所选时间范围内没有事件单" />
    <template v-else>
      <el-row :gutter="16">
        <el-col :xs="24" :lg="14" class="col">
          <el-card shadow="never" :header="`达标率（${groupBy === 'vendor' ? '按厂商' : '按系统'}）`">
            <EChart :option="rateOption" />
          </el-card>
        </el-col>
        <el-col :xs="24" :lg="10" class="col">
          <el-card shadow="never" header="平均恢复时间 MTTR（分钟）">
            <EChart :option="mttrOption" />
          </el-card>
        </el-col>
      </el-row>

      <!-- 表格视图：图表数据的文字版（可读性、可复制、色觉障碍友好） -->
      <el-card shadow="never" header="明细">
        <el-table :data="groups" size="small">
          <el-table-column :label="groupBy === 'vendor' ? '厂商' : '系统'" min-width="160">
            <template #default="{ row }">
              {{ groupBy === 'system' ? `${row.key} ${row.name}` : row.name }}
              <el-tag v-if="isGap(row)" type="warning" size="small">台账缺失</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="tickets" label="故障数" width="80" align="right" />
          <el-table-column prop="resolved" label="已恢复" width="80" align="right" />
          <el-table-column label="MTTR" width="120" align="right">
            <template #default="{ row }">{{ formatMinutes(row.mttr_minutes) }}</template>
          </el-table-column>
          <el-table-column label="响应达标率" width="140" align="right">
            <template #default="{ row }">{{ ratio(row.response_rate, row.response_met, row.response_total) }}</template>
          </el-table-column>
          <el-table-column label="恢复达标率" width="140" align="right">
            <template #default="{ row }">{{ ratio(row.resolve_rate, row.resolve_met, row.resolve_total) }}</template>
          </el-table-column>
          <el-table-column label="当前超时" width="100" align="right">
            <template #default="{ row }">
              <el-tag v-if="row.overdue_now" type="danger" size="small">{{ row.overdue_now }} 单</el-tag>
              <span v-else class="muted">0</span>
            </template>
          </el-table-column>
        </el-table>
        <div class="muted note">"-" 表示该项还没有结论：工单尚未受理/恢复，但也还没到截止时间，不计入达标率分母。</div>
      </el-card>
    </template>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 16px;
}
.block {
  margin-bottom: 16px;
}
.muted {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.note {
  margin-top: 8px;
}
.col {
  margin-bottom: 16px;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}
.tile {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 16px;
}
.tile-label {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.tile-value {
  margin: 8px 0 4px;
  font-size: 26px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  font-variant-numeric: tabular-nums;
}
.tile-sub {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
.clickable-tile {
  cursor: pointer;
}
/* 超时：边框与图标用危险色，数字仍用文字色 */
.critical {
  border-color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}
.critical .tile-label {
  color: var(--el-color-danger);
}
</style>
