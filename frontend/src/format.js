// 展示用的格式化函数。后端时间一律是 UTC（RFC3339），这里统一转成北京时间显示。

const timeFmt = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

/** "2026-10-30T07:06:54Z" → "2026/10/30 15:06:54"；空值显示 "-" */
export function formatTime(iso) {
  return iso ? timeFmt.format(new Date(iso)) : '-'
}

/** 分钟数 → "1 小时 30 分" / "45 分"；null 显示 "-" */
export function formatMinutes(min) {
  if (min == null) return '-'
  const m = Math.round(min)
  const h = Math.floor(m / 60)
  return h > 0 ? `${h} 小时 ${m % 60} 分` : `${m} 分`
}

/** 0.256 → "25.6%"；null（无结论）显示 "-" */
export function formatPercent(ratio) {
  return ratio == null ? '-' : `${(ratio * 100).toFixed(1)}%`
}

/** 系统重要等级 → 显示文字和 el-tag 颜色 */
export function levelTag(level) {
  return (
    { 1: { text: '一级', type: 'danger' }, 2: { text: '二级', type: 'warning' }, 3: { text: '三级', type: 'info' } }[level] ?? {
      text: `${level}级`,
      type: 'info',
    }
  )
}

/** 合同到期提醒：已到期标红，90 天内到期标黄（提前一个季度准备续签），否则不提示（返回 null） */
export function contractTag(dateStr, now = new Date()) {
  if (!dateStr) return null
  const days = Math.ceil((new Date(`${dateStr}T23:59:59+08:00`) - now) / 86400000)
  if (days < 0) return { text: `已到期 ${-days} 天`, type: 'danger' }
  if (days <= 90) return { text: `${days} 天后到期`, type: 'warning' }
  return null
}

/** 工单状态 → 显示文字和 el-tag 颜色 */
export function statusTag(status) {
  return (
    {
      new: { text: '待受理', type: 'danger' },
      acknowledged: { text: '处理中', type: 'warning' },
      resolved: { text: '已恢复', type: 'success' },
      reopened: { text: '已重开', type: 'danger' },
      closed: { text: '已关闭', type: 'info' },
    }[status] ?? { text: status, type: 'info' }
  )
}

/** 严重级别 → el-tag 颜色 */
export function severityType(sev) {
  return { P1: 'danger', P2: 'warning', P3: 'primary', P4: 'info' }[sev] ?? 'info'
}

/** 工单流水动作 → 中文 */
export const actionText = {
  created: '建单',
  duplicate_alert: '告警重复推送',
  alert_resolved: '告警已恢复',
  ack: '受理',
  resolve: '标记恢复',
  close: '确认关闭',
  reopen: '重开',
}

/** 每个状态下允许的操作（与后端状态机一致；只用于决定显示哪些按钮，最终以后端校验为准） */
export const allowedActions = {
  new: ['ack'],
  acknowledged: ['resolve'],
  reopened: ['resolve'],
  resolved: ['close', 'reopen'],
  closed: [],
}

/**
 * 一个时限的当前状态：
 *   已完成 → "按时" / "超时 X"
 *   未完成 → "剩余 X" / "已超时 X"
 */
export function deadlinePhase(deadlineISO, doneISO, now = new Date()) {
  const deadline = new Date(deadlineISO)
  if (doneISO) {
    const late = (new Date(doneISO) - deadline) / 60000
    return late <= 0 ? { text: '按时', type: 'success' } : { text: `超时 ${formatMinutes(late)}`, type: 'danger' }
  }
  const left = (deadline - now) / 60000
  return left >= 0 ? { text: `剩余 ${formatMinutes(left)}`, type: 'warning' } : { text: `已超时 ${formatMinutes(-left)}`, type: 'danger' }
}
