<script setup lang="ts">
import UiBadge from '@/components/ui/UiBadge.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import { formatEmpty, formatEuros, formatShortDate, isoTitle } from '@/utils/format'
import type { FinanceRevenueRecognitionResponse } from '@/api/types/finance'

defineProps<{ report: FinanceRevenueRecognitionResponse }>()

function issueLabel(reason: string): string {
  switch (reason) {
    case 'missing_check_in':
      return 'Missing check-in'
    case 'missing_check_out':
      return 'Missing checkout'
    case 'invalid_check_in':
      return 'Invalid check-in'
    case 'invalid_check_out':
      return 'Invalid checkout'
    case 'invalid_stay_window':
      return 'Invalid stay window'
    default:
      return reason.replaceAll('_', ' ')
  }
}
</script>

<template>
  <div>
    <UiSection
      title="Gross revenue recognition"
      description="Paid Booking.com gross revenue allocated evenly across checkout-exclusive stay nights. Cash transactions remain on their payout dates."
    >
      <div class="revenue-kpi">
        <UiKpiCard
          label="Recognized gross"
          :value="formatEuros(report.gross_revenue_cents)"
          :hint="`${report.bookings.length} payout-backed booking${report.bookings.length === 1 ? '' : 's'}`"
          tone="success"
          hero
        />
      </div>

      <UiInlineBanner
        v-if="report.excluded_bookings.length"
        tone="warning"
        :title="`${report.excluded_bookings.length} payout-backed booking${report.excluded_bookings.length === 1 ? '' : 's'} excluded because the stay window is invalid.`"
      />

      <UiTable
        sticky-header
        :empty="!report.bookings.length"
        empty-text="No payout-backed gross revenue is recognized for this month."
      >
        <template #head>
          <tr>
            <th scope="col">Stay</th>
            <th scope="col">Stay dates</th>
            <th scope="col">Recognized nights</th>
            <th scope="col">Status</th>
            <th scope="col" class="num">Stay gross</th>
            <th scope="col" class="num">Recognized gross</th>
          </tr>
        </template>
        <tr v-for="booking in report.bookings" :key="booking.booking_id">
          <td>
            <strong>{{ formatEmpty(booking.guest_name, `Booking #${booking.booking_id}`) }}</strong>
            <div class="revenue-ref">{{ booking.reference_number }}</div>
          </td>
          <td>
            <time :datetime="booking.check_in_date" :title="isoTitle(booking.check_in_date)">
              {{ formatShortDate(booking.check_in_date) }}
            </time>
            →
            <time :datetime="booking.check_out_date" :title="isoTitle(booking.check_out_date)">
              {{ formatShortDate(booking.check_out_date) }}
            </time>
          </td>
          <td>
            <span v-if="booking.cancelled || booking.no_show">Scheduled check-in month</span>
            <span v-else>{{ booking.recognized_nights }} of {{ booking.stay_nights }}</span>
          </td>
          <td>
            <div class="revenue-flags">
              <UiBadge v-if="booking.unmatched" tone="warning" size="sm">Unmatched</UiBadge>
              <UiBadge v-if="booking.cancelled" tone="info" size="sm">Cancellation charge</UiBadge>
              <UiBadge v-if="booking.no_show" tone="info" size="sm">No-show charge</UiBadge>
              <UiBadge
                v-if="!booking.unmatched && !booking.cancelled && !booking.no_show"
                tone="success"
                size="sm"
                >Recognized</UiBadge
              >
            </div>
          </td>
          <td class="num">{{ formatEuros(booking.gross_cents) }}</td>
          <td class="num">
            <strong>{{ formatEuros(booking.recognized_gross_cents) }}</strong>
          </td>
        </tr>
      </UiTable>
    </UiSection>

    <UiSection
      v-if="report.excluded_bookings.length"
      title="Revenue data issues"
      description="These payout-backed bookings are excluded from every recognized-revenue total until their stay dates are corrected by a later import."
    >
      <UiTable sticky-header>
        <template #head>
          <tr>
            <th scope="col">Booking</th>
            <th scope="col">Check-in</th>
            <th scope="col">Checkout</th>
            <th scope="col">Issue</th>
          </tr>
        </template>
        <tr v-for="issue in report.excluded_bookings" :key="issue.booking_id">
          <td>
            <strong>{{ formatEmpty(issue.guest_name, `Booking #${issue.booking_id}`) }}</strong>
            <div class="revenue-ref">{{ issue.reference_number }}</div>
          </td>
          <td>{{ formatEmpty(issue.check_in_date) }}</td>
          <td>{{ formatEmpty(issue.check_out_date) }}</td>
          <td>
            <UiBadge tone="warning" size="sm">{{ issueLabel(issue.reason) }}</UiBadge>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.revenue-kpi {
  max-width: 320px;
  margin-bottom: var(--space-4);
}

.revenue-ref {
  margin-top: var(--space-1);
  color: var(--color-text-muted);
  font-family: var(--font-mono);
  font-size: var(--font-size-xs);
}

.revenue-flags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}
</style>
