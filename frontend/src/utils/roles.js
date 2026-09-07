export const ROLE_ADMIN = 'admin'
export const ROLE_USER = 'user'

export const ROLE_OPTIONS = [
  { title: 'Admin', value: ROLE_ADMIN },
  { title: 'User', value: ROLE_USER },
]

export function roleLabel(role) {
  if (role === ROLE_ADMIN) return 'Administrator'
  if (role === ROLE_USER) return 'User'
  return role || 'User'
}
