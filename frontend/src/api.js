// 后端 REST API 的薄封装。所有请求走相对路径，开发时由 Vite 代理到后端。

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message)
    this.status = status // HTTP 状态码，如 409
    this.code = code // 后端返回的错误码，如 invalid_transition
  }
}

async function request(method, path, body) {
  const resp = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (resp.status === 204) return null
  const data = await resp.json().catch(() => null)
  if (!resp.ok) {
    // 后端统一的错误体：{"error": "...", "code": "..."}
    if (data?.error) throw new ApiError(resp.status, data.code, data.error)
    // 没有错误体的 502/503/504 通常是代理连不上后端
    const msg = resp.status >= 502 && resp.status <= 504 ? `无法连接后端服务（HTTP ${resp.status}）` : `HTTP ${resp.status}`
    throw new ApiError(resp.status, 'unavailable', msg)
  }
  return data
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body ?? {}),
  put: (path, body) => request('PUT', path, body ?? {}),
  del: (path) => request('DELETE', path),
}
