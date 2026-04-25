<script setup lang="ts">
import IllustrationBase from './IllustrationBase.vue'
interface Props {
  ariaLabel?: string
}
withDefaults(defineProps<Props>(), { ariaLabel: '' })
</script>

<template>
  <!--
    A door with a key hanging from the handle — greets users on the Login
    hero. Key bow and teeth get the accent colour.
  -->
  <IllustrationBase :aria-label="ariaLabel">
    <!-- Door frame -->
    <path d="M36 14 H82 V86 H36 Z" />
    <!-- Door panel -->
    <path d="M44 22 H74 V78 H44 Z" />
    <!-- Handle -->
    <circle cx="68" cy="50" r="2.5" />
    <!-- Key -->
    <circle cx="96" cy="46" r="6" stroke="var(--color-primary)" />
    <path d="M96 52 V78" stroke="var(--color-primary)" />
    <path d="M96 68 H102" stroke="var(--color-primary)" />
    <path d="M96 74 H100" stroke="var(--color-primary)" />
    <!-- Chain to handle -->
    <path d="M90 46 Q78 48 68 50" opacity="0.5" stroke-dasharray="2 3" />
  </IllustrationBase>
</template>
