<script setup lang="ts">
import UiDialog from '@/components/ui/UiDialog.vue'
import UiButton from '@/components/ui/UiButton.vue'
import type { NukiPinReveal as PinReveal } from '@/api/types/nuki'

defineProps<{
  reveal: PinReveal | null
  secondsLeft: number
}>()

const emit = defineEmits<{ copy: []; close: [] }>()
</script>

<template>
  <UiDialog
    :open="!!reveal"
    :title="reveal ? `PIN — ${reveal.label}` : 'Reveal PIN'"
    size="sm"
    persistent
  >
    <template v-if="reveal">
      <p class="reveal__stay">
        <template v-if="reveal.stayName">
          Stay: <strong>{{ reveal.stayName }}</strong>
        </template>
      </p>
      <div class="reveal__pin" aria-live="polite">
        <code class="reveal__pin-value">{{ reveal.pin }}</code>
      </div>
      <div class="reveal__meta">
        <span class="reveal__countdown" aria-live="polite">
          Auto-hides in {{ secondsLeft }} second<span v-if="secondsLeft !== 1">s</span>
        </span>
        <p class="reveal__note">This reveal is recorded in the Nuki audit log.</p>
      </div>
    </template>
    <template #footer>
      <UiButton variant="secondary" size="sm" @click="emit('copy')">Copy PIN</UiButton>
      <UiButton variant="primary" size="sm" @click="emit('close')">Hide now</UiButton>
    </template>
  </UiDialog>
</template>

<style scoped>
.reveal__stay {
  margin: 0 0 var(--space-3);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.reveal__pin {
  display: flex;
  justify-content: center;
  padding: var(--space-4) 0;
}
.reveal__pin-value {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-kpi-lg);
  font-weight: 700;
  letter-spacing: 0.1em;
  color: var(--color-text);
  background: var(--color-sunken);
  border-radius: var(--radius-md);
  padding: var(--space-3) var(--space-5);
}
.reveal__meta {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: center;
  text-align: center;
}
.reveal__countdown {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--warning-fg);
}
.reveal__note {
  margin: 0;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
</style>
