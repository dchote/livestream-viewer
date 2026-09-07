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

function apiURL(path) {
  return `${getIngressBase()}${path.replace(/^\//, '')}`
}

function parseBody(text) {
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function apiError(message, status, body) {
  const err = new Error(message)
  err.status = status
  err.body = body
  return err
}

/**
 * Turns a raw response into parsed data, or throws a normalised error.
 *
 * Every transport goes through here — including the XHR upload path — so that
 * session expiry is handled identically no matter which one made the call.
 */
function handleResponse(status, statusText, text, path) {
  const data = parseBody(text)

  if (status === 403 && data?.error === 'password_change_required') {
    if (!path.includes('auth/change-password') && !window.location.pathname.endsWith('/change-password')) {
      window.location.assign(`${getIngressBase()}change-password`)
    }
    throw apiError(data?.message || 'password must be changed before continuing', status, data)
  }

  if (status === 401) {
    clearAuthStorage()
    if (!path.includes('auth/login')) {
      window.location.assign(`${getIngressBase()}login`)
    }
    throw apiError(data?.message || data?.error || 'unauthorized', status, data)
  }

  if (status < 200 || status >= 300) {
    throw apiError(data?.message || data?.error || statusText, status, data)
  }

  return data
}

async function request(method, path, body) {
  const headers = { Accept: 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(apiURL(path), {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  return handleResponse(res.status, res.statusText, await res.text(), path)
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body),
  put: (path, body) => request('PUT', path, body),
  patch: (path, body) => request('PATCH', path, body),
  delete: (path) => request('DELETE', path),
  // XHR rather than fetch: upload progress events have no fetch equivalent.
  upload(path, file, onProgress) {
    return new Promise((resolve, reject) => {
      const form = new FormData()
      form.append('file', file)
      const xhr = new XMLHttpRequest()
      xhr.open('POST', apiURL(path))
      xhr.setRequestHeader('Accept', 'application/json')
      const token = getToken()
      if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      xhr.responseType = 'text'
      if (onProgress && xhr.upload) {
        xhr.upload.onprogress = (ev) => {
          if (ev.lengthComputable) onProgress(ev.loaded / ev.total)
        }
      }
      xhr.onload = () => {
        try {
          resolve(handleResponse(xhr.status, xhr.statusText, xhr.responseText, path))
        } catch (err) {
          reject(err)
        }
      }
      xhr.onerror = () => reject(apiError('upload failed', 0, null))
      xhr.send(form)
    })
  },
}
