<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    An invoice document with a folded corner and a currency mark. Accent
    highlights the amount line so the eye finds it first.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- Paper -->
    <path d="M36 14 H80 L92 26 V86 H36 Z" />
    <path d="M80 14 V26 H92" />
    <!-- Header lines -->
    <path d="M46 36 H72" />
    <path d="M46 44 H82" />
    <!-- Row separators -->
    <path d="M46 56 H82" opacity="0.5" />
    <path d="M46 66 H82" opacity="0.5" />
    <!-- Total (accent) -->
    <path d="M46 76 H66" stroke="var(--color-primary)" />
    <path d="M74 76 H82" stroke="var(--color-primary)" />
    <!-- Currency circle -->
    <circle cx="78" cy="76" r="4" stroke="var(--color-primary)" />
  </IllustrationBase>
</template>
