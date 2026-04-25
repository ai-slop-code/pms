<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    Empty inbox: an open tray with two stacked sheet outlines suggesting
    nothing has landed yet. Primary accent on the top sheet's corner fold.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- Tray -->
    <path d="M18 58 L28 76 H100 L110 58" />
    <path d="M18 58 H40 L46 68 H82 L88 58 H110" />
    <!-- Sheets inside -->
    <rect x="40" y="22" width="48" height="34" rx="2" />
    <path d="M52 32 H76" />
    <path d="M52 40 H72" />
    <path d="M52 48 H68" />
    <!-- Folded corner accent -->
    <path d="M82 22 L88 22 L88 28 Z" stroke="var(--color-primary)" />
  </IllustrationBase>
</template>
