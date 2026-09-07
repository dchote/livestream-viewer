import { getIngressBase } from './ingress'

const TOKEN_KEY = 'lsv-token'
const USER_KEY = 'lsv-user'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
}

export function getStoredUser() {
  try {
    return JSON.parse(localStorage.getItem(USER_KEY) || 'null')
  } catch {
    return null
  }
}

export function setStoredUser(user) {
  if (user) localStorage.setItem(USER_KEY, JSON.stringify(user))
  else localStorage.removeItem(USER_KEY)
}

function clearAuthStorage() {
  setToken(null)
  setStoredUser(null)
}

async function request(method, path, body) {
  const headers = { Accept: 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const base = getIngressBase()
  const url = `${base}${path.replace(/^\//, '')}`
  const res = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 204) {
    return null
  }

  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (res.status === 403 && data?.error === 'password_change_required') {
    if (!path.includes('auth/change-password') && !window.location.pathname.endsWith('/change-password')) {
      window.location.assign(`${base}change-password`)
    }
    const err = new Error(data?.message || 'password must be changed before continuing')
    err.status = res.status
    err.body = data
    throw err
  }

  if (res.status === 401) {
    clearAuthStorage()
    if (!path.includes('auth/login')) {
      window.location.assign(`${base}login`)
    }
    const err = new Error(data?.message || data?.error || 'unauthorized')
    err.status = res.status
    err.body = data
    throw err
  }

  if (!res.ok) {
    const err = new Error(data?.message || data?.error || res.statusText)
    err.status = res.status
    err.body = data
    throw err
  }
  return data
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body),
  put: (path, body) => request('PUT', path, body),
  patch: (path, body) => request('PATCH', path, body),
  delete: (path) => request('DELETE', path),
}
