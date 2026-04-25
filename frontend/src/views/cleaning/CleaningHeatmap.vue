<script setup lang="ts">
import { computed } from 'vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import type { CleaningHeatBucket } from '@/api/types/cleaning'

const props = defineProps<{ buckets: CleaningHeatBucket[] }>()

const nonZero = computed(() => props.buckets.filter((b) => b.count > 0))
const maxCount = computed(() => Math.max(1, ...nonZero.value.map((b) => b.count)))
</script>

<template>
  <UiSection title="Arrival heatmap" description="First-entry distribution by hour for the selected month.">
    <UiCard v-if="nonZero.length">
      <div class="arrival-hbars">
        <div v-for="b in nonZero" :key="b.hour" class="arrival-hbar-row">
          <div class="arrival-hbar-label">{{ String(b.hour).padStart(2, '0') }}:00</div>
          <div class="arrival-hbar-track">
            <div
              class="arrival-hbar-fill"
              :style="{ width: `${Math.max(6, Math.round((b.count / maxCount) * 100))}%` }"
            />
          </div>
          <div class="arrival-hbar-value num">{{ b.count }}</div>
        </div>
      </div>
    </UiCard>
    <UiCard v-else>
      <p class="muted">No arrival entries for this month yet.</p>
    </UiCard>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.arrival-hbars {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.arrival-hbar-row {
  display: grid;
  grid-template-columns: 56px 1fr 48px;
  align-items: center;
  gap: var(--space-3);
}
.arrival-hbar-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-weight: 500;
}
.arrival-hbar-track {
  height: 18px;
  border-radius: 999px;
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  overflow: hidden;
}
.arrival-hbar-fill {
  height: 100%;
  min-width: 6%;
  border-radius: 999px;
  background: var(--color-primary);
}
.arrival-hbar-value {
  font-size: var(--font-size-xs);
  color: var(--color-text);
  font-weight: 600;
  text-align: right;
}
</style>
