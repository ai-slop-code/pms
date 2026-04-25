<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    Magnifying glass over an empty document outline. Accent ring on the
    lens communicates "searched, nothing here".
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- Document -->
    <path d="M32 18 H78 L92 32 V82 H32 Z" />
    <path d="M78 18 V32 H92" />
    <path d="M44 48 H68" />
    <path d="M44 58 H72" />
    <path d="M44 68 H60" />
    <!-- Magnifier -->
    <circle cx="86" cy="60" r="14" stroke="var(--color-primary)" />
    <path d="M96 70 L108 82" stroke="var(--color-primary)" />
  </IllustrationBase>
</template>
