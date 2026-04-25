<script setup lang="ts">
import { RouterLink } from 'vue-router'
import UiCard from '@/components/ui/UiCard.vue'

defineProps<{ actions: Array<{ to: string; label: string }> }>()
</script>

<template>
  <UiCard v-if="actions.length" class="dashboard-actions">
    <template #header>
      <h2 class="dashboard-quick-title">Available areas</h2>
    </template>
    <div class="actions-grid">
      <RouterLink
        v-for="action in actions"
        :key="action.to"
        :to="action.to"
        class="action-chip"
      >
        {{ action.label }}
      </RouterLink>
    </div>
  </UiCard>
</template>

<style scoped>
.dashboard-actions {
  margin-top: var(--space-5);
}
.dashboard-quick-title {
  margin: 0;
  font-size: var(--font-size-h4);
  font-weight: 600;
}
.actions-grid {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
.action-chip {
  display: inline-flex;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text);
  background: var(--color-surface);
  transition: background var(--motion-1) var(--ease-standard),
    border-color var(--motion-1) var(--ease-standard);
}
.action-chip:hover {
  background: var(--color-primary-weak);
  border-color: var(--color-primary);
  color: var(--color-primary);
}
</style>
