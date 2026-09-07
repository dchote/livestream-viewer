export function getIngressBase() {
  const path = window.location.pathname
  const parts = path.split('/').filter(Boolean)
  if (parts.length >= 3 && parts[0] === 'api' && parts[1] === 'hassio_ingress') {
    return `/${parts[0]}/${parts[1]}/${parts[2]}/`
  }
  const baseEl = document.querySelector('base')
  const href = baseEl?.getAttribute('href')
  if (href && href !== './' && href !== '/') {
    return href.endsWith('/') ? href : `${href}/`
  }
  return '/'
}

export function apiUrl(path) {
  const base = getIngressBase()
  const p = path.startsWith('/') ? path.slice(1) : path
  return `${base}${p}`
}
