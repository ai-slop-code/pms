<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    Warning triangle with an exclamation mark and a subtle shadow line.
    Accent on the exclamation stroke to anchor attention.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <path d="M64 18 L104 82 H24 Z" />
    <path d="M64 38 V58" stroke="var(--color-primary)" />
    <circle cx="64" cy="68" r="2.5" fill="var(--color-primary)" stroke="var(--color-primary)" />
    <!-- Ground line -->
    <path d="M20 88 H108" opacity="0.5" />
  </IllustrationBase>
</template>
