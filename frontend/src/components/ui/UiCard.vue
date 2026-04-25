<script setup lang="ts">
interface Props {
  tone?: 'default' | 'sunken'
  padding?: 'default' | 'tight' | 'none'
}
withDefaults(defineProps<Props>(), {
  tone: 'default',
  padding: 'default',
})
</script>

<template>
  <section
    class="ui-card"
    :class="[`ui-card--tone-${tone}`, `ui-card--pad-${padding}`]"
  >
    <header v-if="$slots.header" class="ui-card__header">
      <slot name="header" />
    </header>
    <div class="ui-card__body">
      <slot />
    </div>
    <footer v-if="$slots.footer" class="ui-card__footer">
      <slot name="footer" />
    </footer>
  </section>
</template>

<style scoped>
.ui-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-0);
}
.ui-card--tone-sunken {
  background: var(--color-sunken);
}
.ui-card--pad-default {
  padding: var(--space-4) var(--space-5);
}
.ui-card--pad-tight {
  padding: var(--space-3) var(--space-4);
}
.ui-card--pad-none {
  padding: 0;
}
.ui-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding-bottom: var(--space-3);
  margin-bottom: var(--space-3);
  border-bottom: 1px solid var(--color-border);
}
.ui-card--pad-none .ui-card__header {
  padding: var(--space-3) var(--space-4);
  margin-bottom: 0;
}
.ui-card__footer {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
  padding-top: var(--space-3);
  margin-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}
</style>
