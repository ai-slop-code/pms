<script setup lang="ts">
import { RouterLink } from 'vue-router'
import UiCard from '@/components/ui/UiCard.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import { formatShortDate, formatShortDateTime, isoTitle } from '@/utils/format'
import { displayStatus, statusTone } from './status'
import type { UpcomingStayWidget } from '@/api/types/dashboard'
import './listRows.css'

defineProps<{ stays: UpcomingStayWidget[] }>()

const fmt = formatShortDateTime
const fmtDay = formatShortDate
</script>

<template>
  <UiCard>
    <template #header>
      <div class="dashboard-widget-head">
        <h2 class="dashboard-widget-title">Upcoming stays</h2>
        <RouterLink to="/occupancy" class="dashboard-widget-link">Open</RouterLink>
      </div>
    </template>
    <UiEmptyState
      v-if="!stays.length"
      illustration="inbox"
      title="No upcoming stays"
      description="Nothing scheduled for this property."
    />
    <ul v-else class="dashboard-list-rows">
      <li v-for="stay in stays" :key="stay.occupancy_id" class="dashboard-list-row">
        <div>
          <div class="dashboard-list-row__title">{{ stay.summary || 'Unnamed stay' }}</div>
          <div class="dashboard-list-row__meta">
            <time :datetime="stay.start_at" :title="isoTitle(stay.start_at)">{{ fmt(stay.start_at) }}</time>
            —
            <time :datetime="stay.end_at" :title="isoTitle(stay.end_at)">{{ fmt(stay.end_at) }}</time>
          </div>
        </div>
        <div class="dashboard-list-row__side">
          <UiBadge tone="neutral" :label="fmtDay(stay.start_at)" size="sm" />
          <UiBadge :tone="statusTone(stay.status)" :label="displayStatus(stay.status)" />
        </div>
      </li>
    </ul>
  </UiCard>
</template>
