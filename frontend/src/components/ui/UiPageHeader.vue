<script setup lang="ts">
interface Props {
  title: string
  lede?: string
}
defineProps<Props>()
</script>

<template>
  <header class="ui-page-header">
    <div class="ui-page-header__row">
      <div class="ui-page-header__titles">
        <h1 class="ui-page-header__title">{{ title }}</h1>
        <p v-if="lede" class="ui-page-header__lede">{{ lede }}</p>
      </div>
      <div v-if="$slots.actions" class="ui-page-header__actions">
        <slot name="actions" />
      </div>
    </div>
    <div v-if="$slots.meta" class="ui-page-header__meta">
      <slot name="meta" />
    </div>
  </header>
</template>

<style scoped>
.ui-page-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-bottom: var(--space-5);
}
.ui-page-header__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}
.ui-page-header__titles {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}
.ui-page-header__title {
  font-size: var(--font-size-h1);
  font-weight: 700;
  line-height: var(--line-height-tight);
}
.ui-page-header__lede {
  font-size: var(--font-size-md);
  color: var(--color-text-muted);
  max-width: 70ch;
}
.ui-page-header__actions {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}
.ui-page-header__meta {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}
</style>
