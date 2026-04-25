import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore, type PermissionLevel, type PropertyModule, type Role } from './auth'

type MinimalProperty = { id: number; owner_user_id: number }

function setup(role: Role, userID = 42, permissions: { property_id: number; module: PropertyModule; permission_level: PermissionLevel }[] = []) {
  const store = useAuthStore()
  // Intentionally patch private state through the public refs exposed by the store.
  store.user = { id: userID, email: 'u@example.com', role }
  store.propertyPermissions = permissions.map((p, idx) => ({ id: idx + 1, ...p }))
  return store
}

describe('auth store canAccessPropertyModule', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('returns false when no user is signed in', () => {
    const store = useAuthStore()
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 1 } as MinimalProperty, 'occupancy', 'read')).toBe(false)
  })

  it('super_admin bypasses all checks', () => {
    const store = setup('super_admin', 99)
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 7 } as MinimalProperty, 'nuki_access', 'admin')).toBe(true)
  })

  it('property owner always has admin-level access', () => {
    const store = setup('owner', 7)
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 7 } as MinimalProperty, 'finance', 'admin')).toBe(true)
  })

  it('grants access when the permission level meets the minimum rank', () => {
    const store = setup('property_manager', 7, [
      { property_id: 1, module: 'finance', permission_level: 'write' },
    ])
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 99 }, 'finance', 'read')).toBe(true)
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 99 }, 'finance', 'write')).toBe(true)
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 99 }, 'finance', 'admin')).toBe(false)
  })

  it('denies access for a different module even when the user has others', () => {
    const store = setup('property_manager', 7, [
      { property_id: 1, module: 'finance', permission_level: 'admin' },
    ])
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 99 }, 'nuki_access', 'read')).toBe(false)
  })

  it('denies access for an unrelated property', () => {
    const store = setup('property_manager', 7, [
      { property_id: 2, module: 'finance', permission_level: 'admin' },
    ])
    expect(store.canAccessPropertyModule({ id: 1, owner_user_id: 99 }, 'finance', 'read')).toBe(false)
  })

  it('returns false when the property is missing', () => {
    const store = setup('owner', 7)
    expect(store.canAccessPropertyModule(null, 'finance', 'read')).toBe(false)
  })
})
