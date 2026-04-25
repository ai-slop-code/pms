<script setup lang="ts">
interface Props {
  title?: string
  description?: string
}
defineProps<Props>()
</script>

<template>
  <section class="ui-section">
    <header v-if="title || $slots.actions || description" class="ui-section__header">
      <div class="ui-section__titles">
        <h2 v-if="title" class="ui-section__title">{{ title }}</h2>
        <p v-if="description" class="ui-section__description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-section__actions">
        <slot name="actions" />
      </div>
    </header>
    <div class="ui-section__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.ui-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.ui-section + .ui-section {
  margin-top: var(--space-6);
}

.ui-section__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}
.ui-section__titles {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.ui-section__title {
  font-size: var(--font-size-h2);
  font-weight: 600;
  line-height: var(--line-height-tight);
  color: var(--color-text);
}
.ui-section__description {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  max-width: 70ch;
}
.ui-section__actions {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}
.ui-section__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>
