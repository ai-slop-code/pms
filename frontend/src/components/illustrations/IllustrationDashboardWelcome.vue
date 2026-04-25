<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    A tidy house with a waving pennant on top — the zero-properties
    "welcome" illustration on the Dashboard.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- House body -->
    <path d="M28 80 V46 L64 22 L100 46 V80 Z" />
    <!-- Door -->
    <path d="M56 80 V60 H72 V80" />
    <path d="M68 70 H70" />
    <!-- Window -->
    <rect x="36" y="54" width="12" height="12" />
    <!-- Flag pole + pennant (accent) -->
    <path d="M64 22 V10" stroke="var(--color-primary)" />
    <path d="M64 12 L76 16 L64 20 Z" fill="var(--color-primary)" stroke="var(--color-primary)" />
    <!-- Ground -->
    <path d="M18 86 H110" opacity="0.5" />
  </IllustrationBase>
</template>
