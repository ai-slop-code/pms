import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '@/api/http'
import type { Property } from '@/stores/property'

export type Role = 'super_admin' | 'owner' | 'property_manager' | 'read_only'
export type PermissionLevel = 'read' | 'write' | 'admin'
export type PropertyModule =
  | 'property_settings'
  | 'occupancy'
  | 'nuki_access'
  | 'cleaning_log'
  | 'finance'
  | 'invoices'
  | 'messages'
  | 'analytics'
  | 'users_permissions'

export interface User {
  id: number
  email: string
  role: Role
  /**
   * Set to true when the backend's provisioning gate is forcing the user
   * to rotate their password (bootstrap super_admin, admin reset, etc.).
   * The router guard funnels these accounts to /provisioning.
   */
  must_change_password?: boolean
}

export interface PropertyPermission {
  id: number
  property_id: number
  module: PropertyModule
  permission_level: PermissionLevel
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const propertyPermissions = ref<PropertyPermission[]>([])
  const loaded = ref(false)
  // mfaPending is true when the password has been accepted but the TOTP
  // challenge has not. The UI keeps the login screen visible in this
  // state and swaps in the code-entry form.
  const mfaPending = ref(false)
  // twoFactorEnrolled tracks whether the current user has finished TOTP
  // enrolment. null means "unknown / not yet fetched". Populated by
  // fetchTwoFactorStatus(), which the auth flow calls automatically after
  // login / refreshMe / 2FA verify so the provisioning view can decide
  // which stage to render without each component having to refetch.
  const twoFactorEnrolled = ref<boolean | null>(null)
  // provisioningRequired mirrors the backend's ProvisioningGate decision
  // for the current user. Sourced from `/api/auth/me` so the SPA cannot
  // disagree with the server (e.g. when PMS_2FA_DEV_BYPASS is on, the
  // backend waives the super-admin enrolment requirement).
  const provisioningRequired = ref(false)

  const isSuperAdmin = computed(() => user.value?.role === 'super_admin')

  function permissionRank(level?: PermissionLevel) {
    switch (level) {
      case 'read':
        return 1
      case 'write':
        return 2
      case 'admin':
        return 3
      default:
        return 0
    }
  }

  async function refreshPermissions() {
    if (!user.value) {
      propertyPermissions.value = []
      return
    }
    const r = await api<{ property_permissions: PropertyPermission[] }>(`/api/users/${user.value.id}`)
    propertyPermissions.value = r.property_permissions
  }

  /**
   * Fetch /api/auth/2fa/status and cache `enrolled` so provisioningRequired
   * stays accurate across navigations. The endpoint is on the provisioning
   * gate's allow-list, so it works even before the user has rotated their
   * bootstrap password. Failures (e.g. transient network) leave the value
   * untouched rather than racing the user to the wrong screen.
   */
  async function fetchTwoFactorStatus() {
    if (!user.value) {
      twoFactorEnrolled.value = null
      return
    }
    try {
      const r = await api<{ enrolled: boolean }>('/api/auth/2fa/status')
      twoFactorEnrolled.value = r.enrolled
    } catch {
      // Leave twoFactorEnrolled as-is so a flaky call doesn't bounce the
      // user back to /provisioning if they were already through it.
    }
  }

  function canAccessPropertyModule(
    property: Pick<Property, 'id' | 'owner_user_id'> | null | undefined,
    module: PropertyModule,
    minLevel: PermissionLevel = 'read'
  ) {
    if (!user.value || !property) return false
    if (user.value.role === 'super_admin') return true
    if (property.owner_user_id === user.value.id) return true
    const perm = propertyPermissions.value.find((p) => p.property_id === property.id && p.module === module)
    return permissionRank(perm?.permission_level) >= permissionRank(minLevel)
  }

  async function refreshMe() {
    try {
      const r = await api<{
        user?: User
        mfa_required?: boolean
        provisioning_required?: boolean
      }>('/api/auth/me')
      if (r.mfa_required) {
        user.value = null
        propertyPermissions.value = []
        mfaPending.value = true
        twoFactorEnrolled.value = null
        provisioningRequired.value = false
      } else if (r.user) {
        user.value = r.user
        mfaPending.value = false
        provisioningRequired.value = !!r.provisioning_required
        await refreshPermissions()
        await fetchTwoFactorStatus()
      } else {
        user.value = null
        propertyPermissions.value = []
        mfaPending.value = false
        twoFactorEnrolled.value = null
        provisioningRequired.value = false
      }
    } catch {
      user.value = null
      propertyPermissions.value = []
      mfaPending.value = false
      twoFactorEnrolled.value = null
      provisioningRequired.value = false
    } finally {
      loaded.value = true
    }
  }

  async function login(email: string, password: string) {
    const r = await api<{
      user?: User
      mfa_required?: boolean
      provisioning_required?: boolean
    }>('/api/auth/login', {
      method: 'POST',
      json: { email, password },
    })
    if (r.mfa_required) {
      mfaPending.value = true
      user.value = null
      propertyPermissions.value = []
      twoFactorEnrolled.value = null
      provisioningRequired.value = false
      loaded.value = true
      return
    }
    if (r.user) {
      user.value = r.user
      mfaPending.value = false
      provisioningRequired.value = !!r.provisioning_required
      await refreshPermissions()
      await fetchTwoFactorStatus()
      loaded.value = true
    }
  }

  async function verifyTwoFactor(payload: { code?: string; recovery_code?: string }) {
    const r = await api<{ user?: User; provisioning_required?: boolean }>(
      '/api/auth/2fa/verify',
      {
        method: 'POST',
        json: payload,
      },
    )
    if (r.user) {
      user.value = r.user
      mfaPending.value = false
      provisioningRequired.value = !!r.provisioning_required
      await refreshPermissions()
      await fetchTwoFactorStatus()
      loaded.value = true
    }
  }

  async function logout() {
    await api<unknown>('/api/auth/logout', { method: 'POST' })
    user.value = null
    propertyPermissions.value = []
    mfaPending.value = false
    twoFactorEnrolled.value = null
    provisioningRequired.value = false
  }

  async function twoFactorStatus() {
    return api<{ enrolled: boolean; mfa_pending?: boolean; recovery_codes_remaining: number }>(
      '/api/auth/2fa/status',
    )
  }

  async function twoFactorEnrollStart() {
    return api<{ secret: string; otpauth_url: string }>('/api/auth/2fa/enroll/start', {
      method: 'POST',
    })
  }

  async function twoFactorEnrollConfirm(secret: string, code: string) {
    return api<{ recovery_codes: string[] }>('/api/auth/2fa/enroll/confirm', {
      method: 'POST',
      json: { secret, code },
    })
  }

  async function twoFactorDisable(password: string) {
    await api<unknown>('/api/auth/2fa/disable', {
      method: 'POST',
      json: { password },
    })
  }

  /**
   * Self-service password rotation, used by the /provisioning view to
   * clear `must_change_password`. The backend invalidates every other
   * session for this user but keeps the current one alive so we don't
   * have to re-login mid-flow. We refresh the local user object so the
   * router guard sees the cleared flag immediately.
   */
  async function rotatePassword(newPassword: string) {
    if (!user.value) throw new Error('not signed in')
    await api<unknown>(`/api/users/${user.value.id}`, {
      method: 'PATCH',
      json: { password: newPassword },
    })
    await refreshMe()
  }

  return {
    user,
    propertyPermissions,
    loaded,
    mfaPending,
    twoFactorEnrolled,
    provisioningRequired,
    isSuperAdmin,
    refreshMe,
    refreshPermissions,
    fetchTwoFactorStatus,
    canAccessPropertyModule,
    login,
    verifyTwoFactor,
    logout,
    twoFactorStatus,
    twoFactorEnrollStart,
    twoFactorEnrollConfirm,
    twoFactorDisable,
    rotatePassword,
  }
})
