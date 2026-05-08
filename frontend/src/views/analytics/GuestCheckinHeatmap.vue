<script setup lang="ts">
// PMS_12 task #3 — guest check-in time heatmap.
// Renders a 24-bucket horizontal bar chart of the hour-of-day at which
// guests first unlocked the smartlock. Mirrors the cleaning arrival
// heatmap visually for consistency, but is fed by a separate analytics
// endpoint and uses (occupancy, day) granularity so multi-stay days are
// counted independently.
import { computed } from 'vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import type { GuestCheckinHeatmapResponse } from '@/api/types/analytics'

const props = defineProps<{ heatmap: GuestCheckinHeatmapResponse | null }>()

const nonZero = computed(() => (props.heatmap?.buckets ?? []).filter((b) => b.count > 0))
const maxCount = computed(() => Math.max(1, ...nonZero.value.map((b) => b.count)))
const totalEntries = computed(() =>
  (props.heatmap?.buckets ?? []).reduce((acc, b) => acc + b.count, 0),
)
</script>

<template>
  <UiSection
    title="Guest check-in times"
    description="Hour-of-day distribution of the first guest unlock per stay per day, derived from Nuki keypad events."
  >
    <UiCard v-if="totalEntries > 0">
      <div
        class="guest-checkin-bars"
        role="img"
        :aria-label="`Guest check-in time distribution across 24 hours, ${totalEntries} unlocks`"
      >
        <div v-for="b in nonZero" :key="b.hour" class="guest-checkin-row">
          <div class="guest-checkin-label">{{ String(b.hour).padStart(2, '0') }}:00</div>
          <div class="guest-checkin-track">
            <div
              class="guest-checkin-fill"
              :style="{ width: `${Math.max(6, Math.round((b.count / maxCount) * 100))}%` }"
            />
          </div>
          <div class="guest-checkin-value num">{{ b.count }}</div>
        </div>
      </div>
      <table class="sr-only">
        <caption>Guest check-in counts by hour of day.</caption>
        <thead><tr><th scope="col">Hour</th><th scope="col">Unlocks</th></tr></thead>
        <tbody>
          <tr v-for="b in heatmap?.buckets ?? []" :key="b.hour">
            <th scope="row">{{ String(b.hour).padStart(2, '0') }}:00</th>
            <td>{{ b.count }}</td>
          </tr>
        </tbody>
      </table>
    </UiCard>
    <UiCard v-else>
      <p class="muted">
        No guest check-ins recorded for this range yet. The reconciler refreshes
        every cleaning interval; new Nuki unlocks usually appear within minutes.
      </p>
    </UiCard>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.guest-checkin-bars {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.guest-checkin-row {
  display: grid;
  grid-template-columns: 56px 1fr 48px;
  align-items: center;
  gap: var(--space-3);
}
.guest-checkin-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-weight: 500;
}
.guest-checkin-track {
  height: 18px;
  border-radius: 999px;
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  overflow: hidden;
}
.guest-checkin-fill {
  height: 100%;
  min-width: 6%;
  border-radius: 999px;
  background: var(--color-primary);
}
.guest-checkin-value {
  font-size: var(--font-size-xs);
  color: var(--color-text);
  font-weight: 600;
  text-align: right;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
