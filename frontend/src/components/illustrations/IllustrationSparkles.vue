<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    A spray bottle surrounded by sparkle dots — used on the Cleaning log
    empty state. Sparkles get the accent colour.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- Bottle -->
    <path d="M54 36 H74 V78 Q74 84 68 84 H60 Q54 84 54 78 Z" />
    <!-- Neck + cap -->
    <path d="M60 28 H68 V36 H60 Z" />
    <!-- Nozzle -->
    <path d="M68 28 L80 22 L82 28" />
    <!-- Label -->
    <path d="M58 48 H70 V62 H58 Z" opacity="0.5" />
    <!-- Sparkles (accent) -->
    <g stroke="var(--color-primary)">
      <path d="M26 26 L26 36 M21 31 L31 31" />
      <path d="M98 40 L98 48 M94 44 L102 44" />
      <path d="M34 66 L34 74 M30 70 L38 70" />
      <circle cx="100" cy="72" r="2" fill="var(--color-primary)" />
    </g>
  </IllustrationBase>
</template>
