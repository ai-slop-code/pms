<script setup lang="ts">
/**
 * Internal wrapper for illustration SFCs. Handles the aria contract
 * mandated by PMS_08 §12.2:
 *
 *   - Decorative by default (`aria-hidden="true"`, no role).
 *   - When `ariaLabel` is provided, switch to `role="img"` + the label.
 *
 * Illustration components should mount this element and place their
 * `<path>` / `<g>` children in the default slot. Consumers size the
 * illustration via the surrounding CSS (e.g. `.illustration { width: 140px; }`).
 */
interface Props {
  ariaLabel?: string
  viewBox?: string
}
const props = withDefaults(defineProps<Props>(), {
  ariaLabel: '',
  viewBox: '0 0 128 96',
})
</script>

<template>
  <svg
    :viewBox="props.viewBox"
    :role="props.ariaLabel ? 'img' : undefined"
    :aria-hidden="props.ariaLabel ? undefined : true"
    :aria-label="props.ariaLabel || undefined"
    preserveAspectRatio="xMidYMid meet"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    class="illustration"
  >
    <slot />
  </svg>
</template>

<style scoped>
.illustration {
  display: block;
  width: 100%;
  height: auto;
  max-width: 180px;
  color: var(--color-text-muted);
}
</style>
