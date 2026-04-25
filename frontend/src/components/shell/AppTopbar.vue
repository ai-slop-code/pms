<script setup lang="ts">
import { computed } from 'vue'
import { Menu, LogOut } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import { useAuthStore } from '@/stores/auth'
import { usePropertyStore } from '@/stores/property'

const emit = defineEmits<{ (e: 'toggle-sidebar'): void; (e: 'logout'): void }>()

const auth = useAuthStore()
const propStore = usePropertyStore()

const propertyOptions = computed(() => propStore.list)
</script>

<template>
  <header class="app-topbar" role="banner">
    <button
      type="button"
      class="app-topbar__menu"
      aria-controls="app-sidebar"
      aria-label="Open navigation"
      @click="emit('toggle-sidebar')"
    >
      <Menu :size="22" aria-hidden="true" />
    </button>

    <div class="app-topbar__brand">
      <span class="app-topbar__logo" aria-hidden="true">PMS</span>
      <span class="sr-only">Property Management System</span>
    </div>

    <div class="app-topbar__spacer" />

    <label v-if="propertyOptions.length" class="app-topbar__property">
      <span class="sr-only">Property</span>
      <select
        id="prop-picker-topbar"
        :value="propStore.currentId ?? ''"
        @change="propStore.currentId = Number(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="p in propertyOptions" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
    </label>

    <span v-if="auth.user" class="app-topbar__user">{{ auth.user.email }}</span>

    <UiButton
      variant="ghost"
      size="sm"
      aria-label="Logout"
      @click="emit('logout')"
    >
      <template #iconLeft>
        <LogOut :size="16" aria-hidden="true" />
      </template>
      <span class="app-topbar__logout-label">Logout</span>
    </UiButton>
  </header>
</template>

<style scoped>
.app-topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  height: var(--topbar-height);
  padding: 0 var(--space-4);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
}

.app-topbar__menu {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: var(--radius-md);
  border: 1px solid transparent;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
}
.app-topbar__menu:hover {
  background: var(--color-sunken);
}
@media (min-width: 1024px) {
  .app-topbar__menu {
    display: none;
  }
}

.app-topbar__brand {
  display: inline-flex;
  align-items: center;
  font-weight: 700;
  color: var(--color-text);
  letter-spacing: 0.04em;
}
.app-topbar__logo {
  font-size: 18px;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--color-primary);
  color: #fff;
}

.app-topbar__spacer {
  flex: 1;
}

.app-topbar__property select {
  width: auto;
  min-width: 12rem;
  min-height: 36px;
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  font: var(--font-size-md) / 1 var(--font-family-sans);
  color: var(--color-text);
}

.app-topbar__user {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

@media (max-width: 767.98px) {
  .app-topbar__user {
    display: none;
  }
  .app-topbar__property select {
    min-width: 8rem;
    max-width: 10rem;
  }
  .app-topbar__logout-label {
    display: none;
  }
}
</style>
