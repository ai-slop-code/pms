<script setup lang="ts">
import { RouterLink } from 'vue-router'
import UiCard from '@/components/ui/UiCard.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import { formatShortDateTime, isoTitle } from '@/utils/format'
import { displayStatus, statusTone } from './status'
import type { ActiveNukiCodeWidget } from '@/api/types/dashboard'
import './listRows.css'

defineProps<{ codes: ActiveNukiCodeWidget[] }>()

const fmt = formatShortDateTime
</script>

<template>
  <UiCard>
    <template #header>
      <div class="dashboard-widget-head">
        <h2 class="dashboard-widget-title">Active Nuki codes</h2>
        <RouterLink to="/nuki" class="dashboard-widget-link">Open</RouterLink>
      </div>
    </template>
    <UiEmptyState
      v-if="!codes.length"
      illustration="inbox"
      title="No active codes"
      description="No Nuki access codes currently active."
    />
    <ul v-else class="dashboard-list-rows">
      <li v-for="code in codes" :key="code.occupancy_id" class="dashboard-list-row">
        <div>
          <div class="dashboard-list-row__title">{{ code.summary || code.code_label || 'Code' }}</div>
          <div class="dashboard-list-row__meta">{{ code.code_masked || '—' }}</div>
          <div class="dashboard-list-row__meta">
            Valid
            <time :datetime="code.valid_from || undefined" :title="isoTitle(code.valid_from)">{{ fmt(code.valid_from) }}</time>
            —
            <time :datetime="code.valid_until || undefined" :title="isoTitle(code.valid_until)">{{ fmt(code.valid_until) }}</time>
          </div>
          <div v-if="code.last_updated_at" class="dashboard-list-row__meta">
            Updated
            <time :datetime="code.last_updated_at" :title="isoTitle(code.last_updated_at)">{{ fmt(code.last_updated_at) }}</time>
          </div>
          <div v-if="code.error_message" class="dashboard-list-row__error">
            {{ code.error_message }}
          </div>
        </div>
        <div class="dashboard-list-row__side">
          <UiBadge :tone="statusTone(code.status)" :label="displayStatus(code.status)" />
        </div>
      </li>
    </ul>
  </UiCard>
</template>
