<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ArrowRight } from 'lucide-vue-next'
import { api } from '@/api/http'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import type { User } from '@/api/types/users'

const users = ref<User[]>([])
const error = ref('')
const success = ref('')
const newEmail = ref('')
const newPassword = ref('')
const newRole = ref('owner')
const creating = ref(false)
const loaded = ref(false)

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

function roleTone(role: string): 'info' | 'success' | 'neutral' | 'warning' {
  if (role === 'super_admin') return 'warning'
  if (role === 'property_manager') return 'info'
  if (role === 'owner') return 'success'
  return 'neutral'
}

async function refresh() {
  const r = await api<{ users: User[] }>('/api/users')
  users.value = r.users
}

onMounted(async () => {
  try {
    await refresh()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load users'
  } finally {
    loaded.value = true
  }
})

async function createUser() {
  error.value = ''
  success.value = ''
  creating.value = true
  try {
    await api('/api/users', {
      method: 'POST',
      json: { email: newEmail.value, password: newPassword.value, role: newRole.value },
    })
    newEmail.value = ''
    newPassword.value = ''
    success.value = 'User created.'
    await refresh()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to create user'
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div>
    <UiPageHeader
      title="Users"
      lede="Manage who can access the system and the role they get by default."
    />

    <UiInlineBanner v-if="error" tone="danger" :title="error" />
    <UiInlineBanner v-if="success" tone="success" :title="success" />

    <UiSection title="Create user" description="New users can be assigned per-property module access from their detail page.">
      <UiCard>
        <form class="user-form" @submit.prevent="createUser">
          <UiInput v-model="newEmail" label="Email" type="email" required autocomplete="off" />
          <UiInput
            v-model="newPassword"
            label="Password"
            type="password"
            required
            autocomplete="new-password"
          />
          <UiSelect v-model="newRole" label="Role">
            <option value="owner">Owner</option>
            <option value="property_manager">Property Manager</option>
            <option value="read_only">Read Only</option>
            <option value="super_admin">Super Admin</option>
          </UiSelect>
          <div class="user-form__actions">
            <UiButton type="submit" variant="primary" :loading="creating">Create user</UiButton>
          </div>
        </form>
      </UiCard>
    </UiSection>

    <UiSection title="All users">
      <UiTable :empty="loaded && !users.length" empty-text="No users yet — create the first one above.">
        <template #head>
          <tr>
            <th>Email</th>
            <th>Role</th>
            <th aria-label="Actions" />
          </tr>
        </template>
        <tr v-for="u in users" :key="u.id">
          <td>{{ u.email }}</td>
          <td>
            <UiBadge :tone="roleTone(u.role)">{{ displayRole(u.role) }}</UiBadge>
          </td>
          <td class="row-actions">
            <RouterLink :to="`/users/${u.id}`" class="row-link">
              Permissions
              <ArrowRight :size="14" aria-hidden="true" />
            </RouterLink>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.user-form {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
  align-items: end;
}
.user-form__actions {
  display: flex;
  justify-content: flex-end;
  grid-column: 1 / -1;
}
.row-actions {
  text-align: right;
  white-space: nowrap;
}
.row-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--color-primary);
  font-weight: 500;
}
.row-link:hover {
  text-decoration: none;
  color: var(--color-primary-hover, var(--color-primary));
}
</style>
