// 系统没有登录，操作人由使用者自己填写；记在浏览器本地，下次自动带出。
// 本地存储可能不可用（无痕模式、禁用站点数据），失败时静默退化为每次手填。
const KEY = 'opshub.operator'

export function loadOperator() {
  try {
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

export function saveOperator(name) {
  try {
    localStorage.setItem(KEY, name)
  } catch {
    // 忽略
  }
}
