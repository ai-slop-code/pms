<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { illustrations, type IllustrationName } from '@/components/illustrations/registry'

interface Props {
  title: string
  description?: string
  /**
   * Optional hand-drawn illustration rendered in the icon position.
   * See PMS_08 §12.2. If the `icon` slot is also provided, the slot wins.
   */
  illustration?: IllustrationName
}
const props = defineProps<Props>()
const slots = useSlots()

const illustrationComponent = computed(() =>
  !slots.icon && props.illustration ? illustrations[props.illustration] : null,
)
</script>

<template>
  <div class="ui-empty">
    <div v-if="slots.icon" class="ui-empty__icon" aria-hidden="true"><slot name="icon" /></div>
    <div v-else-if="illustrationComponent" class="ui-empty__illustration" aria-hidden="true">
      <component :is="illustrationComponent" />
    </div>
    <h3 class="ui-empty__title">{{ title }}</h3>
    <p v-if="description" class="ui-empty__description">{{ description }}</p>
    <div v-if="slots.actions" class="ui-empty__actions"><slot name="actions" /></div>
  </div>
</template>

<style scoped>
.ui-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: var(--space-6) var(--space-4);
  gap: var(--space-2);
  color: var(--color-text-muted);
}
.ui-empty__icon {
  color: var(--color-text-subtle);
  margin-bottom: var(--space-1);
}
.ui-empty__illustration {
  color: var(--color-text-subtle);
  width: 140px;
  max-width: 60%;
  margin-bottom: var(--space-2);
}
.ui-empty__title {
  font-size: var(--font-size-h4);
  font-weight: 600;
  color: var(--color-text);
}
.ui-empty__description {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  max-width: 44ch;
}
.ui-empty__actions {
  display: inline-flex;
  gap: var(--space-2);
  margin-top: var(--space-3);
}
</style>
