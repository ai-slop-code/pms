<script setup lang="ts">
defineProps<{ open: boolean }>()
</script>

<template>
  <div v-if="open" class="glossary">
    <dl class="glossary-list">
      <div><dt>ADR</dt><dd>Average Daily Rate — gross revenue ÷ nights sold.</dd></div>
      <div><dt>RevPAR</dt><dd>Revenue per Available Night — gross revenue ÷ available nights.</dd></div>
      <div><dt>Occupancy rate</dt><dd>Nights sold ÷ available nights.</dd></div>
      <div><dt>Effective take rate</dt><dd>(commission + payment fees) ÷ gross — share of revenue lost to platform.</dd></div>
      <div><dt>Confirmed vs. estimated revenue</dt><dd>Confirmed = payouts already matched to stays in the window. Estimated = remaining unsold nights valued at the trailing 90-day ADR.</dd></div>
      <div><dt>Booking pace</dt><dd>Cumulative number of reservations received for a target window, plotted by booking date.</dd></div>
      <div><dt>Gap night</dt><dd>A single vacant night sandwiched between two occupied stays.</dd></div>
      <div><dt>Orphan midweek night</dt><dd>A vacant Mon–Thu night wrapped by occupied weekends on both sides.</dd></div>
      <div><dt>Lead time</dt><dd>Days between the moment a reservation was booked and the arrival date.</dd></div>
      <div><dt>Length of stay</dt><dd>Number of nights per reservation.</dd></div>
    </dl>
  </div>
</template>

<style scoped>
.glossary {
  background: var(--color-sunken);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}
.glossary-list {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-2) var(--space-4);
}
.glossary-list > div {
  display: flex;
  flex-direction: column;
  font-size: var(--font-size-sm);
}
.glossary-list dt {
  font-weight: 600;
  color: var(--color-text);
}
.glossary-list dd {
  margin: 0;
  color: var(--color-text-muted);
}
</style>
