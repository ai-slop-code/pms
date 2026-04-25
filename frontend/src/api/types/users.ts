// Users + permissions API types — /api/users and /api/users/:id/permissions.

export type UserRole = string

export interface User {
  id: number
  email: string
  role: UserRole
}

export interface UserPermission {
  id: number
  property_id: number
  module: string
  permission_level: string
}
