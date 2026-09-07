/**
 * Client-side mirror of the server password policy.
 * Must match `model.MinPasswordLength` in internal/model.
 */
export const MIN_PASSWORD_LENGTH = 8

export const PASSWORD_HINT = `At least ${MIN_PASSWORD_LENGTH} characters`

export const PASSWORD_TOO_SHORT = `Password must be at least ${MIN_PASSWORD_LENGTH} characters`

export function passwordTooShort(value) {
  return (value || '').length < MIN_PASSWORD_LENGTH
}
