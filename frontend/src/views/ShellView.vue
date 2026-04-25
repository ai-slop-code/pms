<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import AppTopbar from '@/components/shell/AppTopbar.vue'
import AppSidebar from '@/components/shell/AppSidebar.vue'
import ToastStack from '@/components/ui/ToastStack.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { APP_NAV_ITEMS } from '@/constants/navigation'
import { useAuthStore, type PropertyModule } from '@/stores/auth'
import { useCurrentProperty } from '@/composables/useCurrentProperty'
import { useDocumentTitle } from '@/composables/useDocumentTitle'

const ROUTE_NAME_TITLES: Record<string, string> = {
  dashboard: 'Dashboard',
  properties: 'Properties',
  'property-new': 'New property',
  'property-detail': 'Property',
  occupancy: 'Occupancy',
  nuki: 'Nuki access',
  cleaning: 'Cleaning',
  finance: 'Finance',
  'finance-booking-payouts': 'Booking payouts',
  invoices: 'Invoices',
  messages: 'Messages',
  analytics: 'Analytics',
  users: 'Users',
  'user-detail': 'User',
  'ui-kit': 'UI kit',
}

const auth = useAuthStore()
const { pid, currentProperty, propertyStore } = useCurrentProperty()
const router = useRouter()
const route = useRoute()

const sidebarOpen = ref(false)
const routeAnnouncement = ref('')

function titleForRoute(): string {
  const name = typeof route.name === 'string' ? route.name : ''
  if (name && ROUTE_NAME_TITLES[name]) return ROUTE_NAME_TITLES[name]
  const navMatch = APP_NAV_ITEMS.find((item) => item.to === route.path)
  if (navMatch) return navMatch.label
  if (name) return name.replace(/[-_]/g, ' ')
  return 'Page'
}

function routeNeedsModuleAccess() {
  return typeof route.meta.module === 'string'
}

function currentRouteAllowed() {
  if (!routeNeedsModuleAccess()) return true
  const property = currentProperty.value
  if (!property) return false
  return auth.canAccessPropertyModule(property, route.meta.module as PropertyModule)
}

async function ensureAllowedCurrentRoute() {
  if (route.path === '/login') return
  if (!currentRouteAllowed()) {
    await router.replace('/')
  }
}

onMounted(() => {
  propertyStore.loadStored()
  if (auth.user) propertyStore.fetchList().catch(() => {})
})

watch(
  () => auth.user,
  (u) => {
    if (u) propertyStore.fetchList().catch(() => {})
  },
)

watch(
  [pid, () => auth.propertyPermissions, () => route.fullPath],
  () => {
    ensureAllowedCurrentRoute().catch(() => {})
    // Auto-close drawer on route change (mobile).
    sidebarOpen.value = false
  },
  { deep: true },
)

watch(
  () => route.fullPath,
  () => {
    // Re-assign forces screen readers to announce even if the value is unchanged.
    routeAnnouncement.value = ''
    requestAnimationFrame(() => {
      routeAnnouncement.value = `Navigated to ${titleForRoute()}`
    })
  },
  { immediate: true },
)

useDocumentTitle(() => titleForRoute())

async function doLogout() {
  await auth.logout()
  await router.push('/login')
}
</script>

<template>
  <div class="app-shell">
    <AppTopbar
      @toggle-sidebar="sidebarOpen = !sidebarOpen"
      @logout="doLogout"
    />
    <div class="app-shell__body">
      <AppSidebar :open="sidebarOpen" @close="sidebarOpen = false" />
      <main id="main-content" class="app-shell__main" tabindex="-1">
        <div class="app-shell__container">
          <RouterView />
        </div>
      </main>
    </div>
    <div class="visually-hidden" role="status" aria-live="polite" aria-atomic="true">
      {{ routeAnnouncement }}
    </div>
    <ToastStack />
    <ConfirmHost />
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
  min-height: 100svh;
  display: flex;
  flex-direction: column;
  background: var(--color-bg);
}
.app-shell__body {
  display: flex;
  flex: 1;
  min-height: 0;
}
.app-shell__main {
  flex: 1;
  min-width: 0;
  padding: var(--space-5) 0;
}
.app-shell__main:focus {
  outline: none;
}
.app-shell__container {
  max-width: var(--content-max-width);
  margin: 0 auto;
  padding: 0 clamp(16px, 4vw, 32px);
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 639.98px) {
  .app-shell__main {
    padding: var(--space-4) 0;
  }
}
</style>
