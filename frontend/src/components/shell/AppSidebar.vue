<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import {
  LayoutDashboard,
  Calendar,
  KeyRound,
  Sparkles,
  MessageSquare,
  Wallet,
  Receipt,
  FileText,
  BarChart3,
  Building2,
  Users,
  X,
} from 'lucide-vue-next'
import { useAuthStore, type PropertyModule } from '@/stores/auth'
import { usePropertyStore } from '@/stores/property'

interface Props {
  open: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ (e: 'close'): void }>()

const auth = useAuthStore()
const propStore = usePropertyStore()
const currentProperty = computed(
  () => propStore.list.find((p) => p.id === propStore.currentId) ?? null,
)

interface Item {
  to: string
  label: string
  icon: unknown
  module?: PropertyModule
  superAdmin?: boolean
}
interface Group {
  label: string
  items: Item[]
}

const groups: Group[] = [
  {
    label: 'Operations',
    items: [
      { to: '/', label: 'Dashboard', icon: LayoutDashboard },
      { to: '/occupancy', label: 'Occupancy', icon: Calendar, module: 'occupancy' },
      { to: '/nuki', label: 'Nuki Access', icon: KeyRound, module: 'nuki_access' },
      { to: '/cleaning', label: 'Cleaning', icon: Sparkles, module: 'cleaning_log' },
      { to: '/messages', label: 'Messages', icon: MessageSquare, module: 'messages' },
    ],
  },
  {
    label: 'Money',
    items: [
      { to: '/finance', label: 'Finance', icon: Wallet, module: 'finance' },
      {
        to: '/finance/booking-payouts',
        label: 'Booking Payouts',
        icon: Receipt,
        module: 'finance',
      },
      { to: '/invoices', label: 'Invoices', icon: FileText, module: 'invoices' },
    ],
  },
  {
    label: 'Insights',
    items: [
      { to: '/analytics', label: 'Analytics', icon: BarChart3, module: 'analytics' },
    ],
  },
  {
    label: 'Admin',
    items: [
      { to: '/properties', label: 'Properties', icon: Building2 },
      { to: '/users', label: 'Users', icon: Users, superAdmin: true },
    ],
  },
]

function itemVisible(item: Item): boolean {
  if (item.superAdmin) return auth.isSuperAdmin
  if (!item.module) return true
  return auth.canAccessPropertyModule(currentProperty.value, item.module)
}

const visibleGroups = computed(() =>
  groups
    .map((g) => ({ ...g, items: g.items.filter(itemVisible) }))
    .filter((g) => g.items.length > 0),
)
</script>

<template>
  <div
    v-if="props.open"
    class="app-sidebar__backdrop"
    aria-hidden="true"
    @click="emit('close')"
  />
  <aside
    id="app-sidebar"
    class="app-sidebar"
    :class="{ 'app-sidebar--open': props.open }"
    :aria-hidden="props.open ? undefined : 'true'"
    aria-label="Primary navigation"
  >
    <div class="app-sidebar__head">
      <button
        type="button"
        class="app-sidebar__close"
        aria-label="Close navigation"
        @click="emit('close')"
      >
        <X :size="18" aria-hidden="true" />
      </button>
    </div>

    <nav class="app-sidebar__nav" aria-label="Modules">
      <div v-for="g in visibleGroups" :key="g.label" class="app-sidebar__group">
        <div class="app-sidebar__group-label">{{ g.label }}</div>
        <ul class="app-sidebar__list">
          <li v-for="item in g.items" :key="item.to">
            <RouterLink
              :to="item.to"
              class="app-sidebar__link"
              active-class="app-sidebar__link--active"
              :exact-active-class="item.to === '/' ? 'app-sidebar__link--active' : ''"
              @click="emit('close')"
            >
              <component :is="item.icon" :size="18" class="app-sidebar__icon" aria-hidden="true" />
              <span>{{ item.label }}</span>
            </RouterLink>
          </li>
        </ul>
      </div>
    </nav>
  </aside>
</template>

<style scoped>
.app-sidebar {
  width: var(--sidebar-width);
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  padding: var(--space-3) 0 var(--space-4);
}

/* Desktop: persistent next to content (handled by ShellView layout). */
@media (min-width: 1024px) {
  .app-sidebar {
    position: sticky;
    top: var(--topbar-height);
    height: calc(100vh - var(--topbar-height));
    flex-shrink: 0;
  }
  .app-sidebar__head {
    display: none;
  }
  .app-sidebar__backdrop {
    display: none;
  }
}

/* Mobile/tablet: drawer. */
@media (max-width: 1023.98px) {
  .app-sidebar {
    position: fixed;
    top: 0;
    bottom: 0;
    left: 0;
    width: 280px;
    max-width: 85vw;
    z-index: 30;
    transform: translateX(-100%);
    transition: transform var(--motion-2) var(--ease-standard);
    box-shadow: var(--shadow-3);
  }
  .app-sidebar--open {
    transform: translateX(0);
  }
  .app-sidebar__backdrop {
    position: fixed;
    inset: 0;
    background: var(--color-scrim);
    z-index: 29;
  }
}

.app-sidebar__head {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding: var(--space-3) var(--space-4);
}
.app-sidebar__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  border: 1px solid transparent;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
}
.app-sidebar__close:hover {
  background: var(--color-sunken);
  color: var(--color-text);
}

.app-sidebar__nav {
  display: flex;
  flex-direction: column;
}
.app-sidebar__group {
  padding: var(--space-3) 0;
}
.app-sidebar__group-label {
  font-size: var(--font-size-2xs);
  font-weight: 700;
  color: var(--color-text-subtle);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  padding: 0 var(--space-4);
  margin-bottom: var(--space-1);
}
.app-sidebar__list {
  list-style: none;
  margin: 0;
  padding: 0 var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.app-sidebar__link {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  min-height: 40px;
  color: var(--color-text);
  font-size: var(--font-size-md);
  font-weight: 500;
  text-decoration: none;
  border-left: 3px solid transparent;
  transition: background var(--motion-1) var(--ease-standard),
    color var(--motion-1) var(--ease-standard);
}
.app-sidebar__link:hover {
  background: var(--color-sunken);
  text-decoration: none;
}
.app-sidebar__link--active {
  background: var(--color-primary-weak);
  color: var(--color-primary);
  border-left-color: var(--color-primary);
}
.app-sidebar__link--active:hover {
  background: var(--color-primary-weak);
}
.app-sidebar__icon {
  flex-shrink: 0;
  color: currentColor;
}
</style>
