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
      const r = await api<{ user?: User; mfa_required?: boolean }>('/api/auth/me')
      if (r.mfa_required) {
        user.value = null
        propertyPermissions.value = []
        mfaPending.value = true
      } else if (r.user) {
        user.value = r.user
        mfaPending.value = false
        await refreshPermissions()
      } else {
        user.value = null
        propertyPermissions.value = []
        mfaPending.value = false
      }
    } catch {
      user.value = null
      propertyPermissions.value = []
      mfaPending.value = false
    } finally {
      loaded.value = true
    }
  }

  async function login(email: string, password: string) {
    const r = await api<{ user?: User; mfa_required?: boolean }>('/api/auth/login', {
      method: 'POST',
      json: { email, password },
    })
    if (r.mfa_required) {
      mfaPending.value = true
      user.value = null
      propertyPermissions.value = []
      loaded.value = true
      return
    }
    if (r.user) {
      user.value = r.user
      mfaPending.value = false
      await refreshPermissions()
      loaded.value = true
    }
  }

  async function verifyTwoFactor(payload: { code?: string; recovery_code?: string }) {
    const r = await api<{ user?: User }>('/api/auth/2fa/verify', {
      method: 'POST',
      json: payload,
    })
    if (r.user) {
      user.value = r.user
      mfaPending.value = false
      await refreshPermissions()
      loaded.value = true
    }
  }

  async function logout() {
    await api<unknown>('/api/auth/logout', { method: 'POST' })
    user.value = null
    propertyPermissions.value = []
    mfaPending.value = false
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

  return {
    user,
    propertyPermissions,
    loaded,
    mfaPending,
    isSuperAdmin,
    refreshMe,
    refreshPermissions,
    canAccessPropertyModule,
    login,
    verifyTwoFactor,
    logout,
    twoFactorStatus,
    twoFactorEnrollStart,
    twoFactorEnrollConfirm,
    twoFactorDisable,
  }
})
