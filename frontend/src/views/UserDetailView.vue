<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { ChevronLeft } from 'lucide-vue-next'
import { api } from '@/api/http'
import { usePropertyStore } from '@/stores/property'
import { useAuthStore } from '@/stores/auth'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import TwoFactorSection from '@/components/security/TwoFactorSection.vue'
import { useConfirm } from '@/composables/useConfirm'
import type { UserPermission } from '@/api/types/users'

const route = useRoute()
const userId = computed(() => Number(route.params.id))
const { confirm } = useConfirm()
const auth = useAuthStore()
const isSelf = computed(() => auth.user?.id === userId.value)

const user = ref<{ id: number; email: string; role: string } | null>(null)
const perms = ref<UserPermission[]>([])
const error = ref('')
const success = ref('')
const loaded = ref(false)
const adding = ref(false)
const propertyStore = usePropertyStore()

const newPerm = ref({
  property_id: 0 as number,
  module: 'occupancy',
  permission_level: 'read',
})

const modules = [
  { value: 'property_settings', label: 'Property Settings' },
  { value: 'occupancy', label: 'Occupancy' },
  { value: 'nuki_access', label: 'Nuki Access' },
  { value: 'cleaning_log', label: 'Cleaning' },
  { value: 'finance', label: 'Finance' },
  { value: 'invoices', label: 'Invoices' },
  { value: 'messages', label: 'Messages' },
  { value: 'users_permissions', label: 'Users & Permissions' },
]

function displayRole(role: string) {
  switch (role) {
    case 'super_admin':
      return 'Super Admin'
    case 'property_manager':
      return 'Property Manager'
    case 'read_only':
      return 'Read Only'
    case 'owner':
      return 'Owner'
    default:
      return role.replaceAll('_', ' ')
  }
}

function levelTone(level: string): 'info' | 'success' | 'warning' | 'neutral' {
  if (level === 'admin') return 'warning'
  if (level === 'write') return 'info'
  if (level === 'read') return 'neutral'
  return 'neutral'
}

function displayLevel(level: string) {
  switch (level) {
    case 'read':
      return 'Read'
    case 'write':
      return 'Write'
    case 'admin':
      return 'Admin'
    default:
      return level.replaceAll('_', ' ')
  }
}

function displayModule(module: string) {
  return modules.find((m) => m.value === module)?.label || module.replaceAll('_', ' ')
}

async function load() {
  error.value = ''
  try {
    const r = await api<{ user: { id: number; email: string; role: string }; property_permissions: UserPermission[] }>(
      `/api/users/${userId.value}`
    )
    user.value = r.user
    perms.value = r.property_permissions
    await propertyStore.fetchList()
    if (!newPerm.value.property_id && propertyStore.list.length) {
      newPerm.value.property_id = propertyStore.list[0]?.id ?? 0
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load user permissions'
  } finally {
    loaded.value = true
  }
}

async function addPerm() {
  error.value = ''
  success.value = ''
  adding.value = true
  try {
    await api(`/api/users/${userId.value}/property-permissions`, {
      method: 'POST',
      json: newPerm.value,
    })
    success.value = 'Permission added.'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to add permission'
  } finally {
    adding.value = false
  }
}

async function removePerm(p: UserPermission) {
  const ok = await confirm({
    title: 'Remove permission',
    message: 'Remove this permission? The user will lose access to the module immediately.',
    confirmLabel: 'Remove',
    tone: 'danger',
  })
  if (!ok) return
  error.value = ''
  success.value = ''
  try {
    await api(`/api/users/${userId.value}/property-permissions/${p.id}`, { method: 'DELETE' })
    success.value = 'Permission removed.'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to remove permission'
  }
}

onMounted(load)
watch(userId, load)
</script>

<template>
  <div>
    <UiPageHeader
      :title="user?.email || 'User'"
      :lede="user ? `Role: ${displayRole(user.role)}` : ''"
    >
      <template #meta>
        <RouterLink to="/users" class="back-link">
          <ChevronLeft :size="14" aria-hidden="true" />
          <span>All users</span>
        </RouterLink>
      </template>
    </UiPageHeader>

    <UiInlineBanner v-if="error" tone="danger" :title="error" />
    <UiInlineBanner v-if="success" tone="success" :title="success" />

    <TwoFactorSection v-if="isSelf" />

    <UiSection
      title="Property permissions"
      description="Property-level overrides for this user. Owners get implicit admin access via ownership."
    >
      <UiTable
        :empty="loaded && !perms.length"
        empty-text="No explicit permissions — access is inherited from property ownership."
      >
        <template #head>
          <tr>
            <th>Property</th>
            <th>Module</th>
            <th>Level</th>
            <th aria-label="Actions" />
          </tr>
        </template>
        <tr v-for="p in perms" :key="p.id">
          <td>{{ propertyStore.list.find((x) => x.id === p.property_id)?.name || p.property_id }}</td>
          <td>{{ displayModule(p.module) }}</td>
          <td>
            <UiBadge :tone="levelTone(p.permission_level)">{{ displayLevel(p.permission_level) }}</UiBadge>
          </td>
          <td class="row-actions">
            <UiButton variant="ghost" size="sm" @click="removePerm(p)">Remove</UiButton>
          </td>
        </tr>
      </UiTable>
    </UiSection>

    <UiSection title="Add permission">
      <UiCard>
        <form class="perm-form" @submit.prevent="addPerm">
          <UiSelect v-model.number="newPerm.property_id" label="Property">
            <option v-for="pr in propertyStore.list" :key="pr.id" :value="pr.id">{{ pr.name }}</option>
          </UiSelect>
          <UiSelect v-model="newPerm.module" label="Module">
            <option v-for="m in modules" :key="m.value" :value="m.value">{{ m.label }}</option>
          </UiSelect>
          <UiSelect v-model="newPerm.permission_level" label="Level">
            <option value="read">Read</option>
            <option value="write">Write</option>
            <option value="admin">Admin</option>
          </UiSelect>
          <div class="perm-form__actions">
            <UiButton type="submit" variant="primary" :loading="adding">Add permission</UiButton>
          </div>
        </form>
      </UiCard>
    </UiSection>
  </div>
</template>

<style scoped>
.back-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.back-link:hover {
  color: var(--color-primary);
  text-decoration: none;
}
.perm-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
  align-items: end;
}
.perm-form__actions {
  display: flex;
  justify-content: flex-end;
  grid-column: 1 / -1;
}
.row-actions {
  text-align: right;
  white-space: nowrap;
}
</style>
