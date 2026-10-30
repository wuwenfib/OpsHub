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
