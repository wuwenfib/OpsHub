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
