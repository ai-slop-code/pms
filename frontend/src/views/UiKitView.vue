<script setup lang="ts">
import { ref } from 'vue'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiSection from '@/components/ui/UiSection.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiIconButton from '@/components/ui/UiIconButton.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiFileInput from '@/components/ui/UiFileInput.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiTag from '@/components/ui/UiTag.vue'
import UiToolbar from '@/components/ui/UiToolbar.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiKpiCard from '@/components/ui/UiKpiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiInlineBanner from '@/components/ui/UiInlineBanner.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiSkeleton from '@/components/ui/UiSkeleton.vue'
import UiDialog from '@/components/ui/UiDialog.vue'
import { Plus, RefreshCw, Trash2 } from 'lucide-vue-next'
import { useToast } from '@/composables/useToast'

const toast = useToast()

const inputValue = ref('')
const inputInvalid = ref('invalid@')
const selectValue = ref('b')
const fileValue = ref<File | null>(null)
const tab = ref('tab1')
const dialogOpen = ref(false)
const persistentDialogOpen = ref(false)
</script>

<template>
  <div>
    <UiPageHeader
      title="UI Kit"
      lede="Every primitive × variant. Dev-only surface — keep visual regressions here."
    >
      <template #actions>
        <UiButton variant="ghost" size="sm" @click="toast.info('Hello from the UI kit')">Toast demo</UiButton>
      </template>
    </UiPageHeader>

    <UiSection title="Buttons" description="Primary, secondary, ghost, danger × sm/md/lg; loading; block; icon-only.">
      <UiCard>
        <div class="row">
          <UiButton variant="primary">Primary</UiButton>
          <UiButton variant="secondary">Secondary</UiButton>
          <UiButton variant="ghost">Ghost</UiButton>
          <UiButton variant="danger">Danger</UiButton>
        </div>
        <div class="row">
          <UiButton variant="primary" size="sm">Small</UiButton>
          <UiButton variant="primary" size="md">Medium</UiButton>
          <UiButton variant="primary" size="lg">Large</UiButton>
        </div>
        <div class="row">
          <UiButton variant="primary" loading>Loading</UiButton>
          <UiButton variant="primary" disabled>Disabled</UiButton>
          <UiButton variant="primary">
            <template #iconLeft><Plus :size="16" aria-hidden="true" /></template>
            Add
          </UiButton>
          <UiIconButton label="Refresh">
            <RefreshCw :size="16" aria-hidden="true" />
          </UiIconButton>
          <UiIconButton label="Secondary action" variant="secondary">
            <Trash2 :size="16" aria-hidden="true" />
          </UiIconButton>
        </div>
        <div class="row">
          <UiButton variant="primary" block>Block button</UiButton>
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Form controls" description="Labels + help + error; aria-describedby; file input.">
      <UiCard>
        <div class="form-row">
          <UiInput v-model="inputValue" label="Email" type="email" placeholder="you@example.com" help="We never share it." />
          <UiInput v-model="inputInvalid" label="Invalid" error="Must be a valid email" />
          <UiSelect v-model="selectValue" label="Pick one">
            <option value="a">Option A</option>
            <option value="b">Option B</option>
            <option value="c">Option C</option>
          </UiSelect>
          <UiFileInput v-model="fileValue" label="Attachment" help="PDF or CSV up to 10 MB." accept=".pdf,.csv" />
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Badges & tags">
      <UiCard>
        <div class="row">
          <UiBadge tone="neutral" label="Neutral" />
          <UiBadge tone="info" label="Info" dot />
          <UiBadge tone="success" label="Success" dot />
          <UiBadge tone="warning" label="Warning" dot />
          <UiBadge tone="danger" label="Danger" dot />
          <UiBadge tone="info" size="sm" label="Small" />
        </div>
        <div class="row">
          <UiTag tone="neutral" label="Filter: all" />
          <UiTag tone="info" label="Active" />
          <UiTag tone="success" label="Paid" />
          <UiTag tone="warning" label="Draft" />
          <UiTag tone="danger" label="Overdue" />
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="KPI cards">
      <div class="kpi-grid">
        <UiKpiCard
label="Revenue (month)" value="€12,480" tone="success" hero
                  :trend="{ direction: 'up', label: '+8.2% vs Feb' }" />
        <UiKpiCard label="Outgoing" value="€4,120" tone="warning" />
        <UiKpiCard label="Net" value="€8,360" :trend="{ direction: 'up', label: '+€760' }" />
        <UiKpiCard label="Occupancy" value="78%" hint="Next 30 days" />
        <UiKpiCard
label="Losses" value="€-240" tone="danger"
                  :trend="{ direction: 'down', label: '−€60' }" />
      </div>
    </UiSection>

    <UiSection title="Toolbar & tabs">
      <UiToolbar>
        <UiInput label="Month" type="month" />
        <UiSelect label="Direction">
          <option value="">All</option>
          <option value="in">Incoming</option>
          <option value="out">Outgoing</option>
        </UiSelect>
        <template #trailing>
          <UiButton variant="secondary">Refresh</UiButton>
          <UiButton variant="primary">Open month</UiButton>
        </template>
      </UiToolbar>
      <UiTabs
        :model-value="tab"
        :tabs="[
          { id: 'tab1', label: 'First' },
          { id: 'tab2', label: 'Second' },
          { id: 'tab3', label: 'Disabled', disabled: true },
        ]"
        aria-label="Demo tabs"
        @update:model-value="(v) => (tab = v as string)"
      />
      <UiCard class="spaced">Active tab: <strong>{{ tab }}</strong></UiCard>
    </UiSection>

    <UiSection title="Table">
      <UiTable sticky-header caption="Sample ledger">
        <template #head>
          <tr><th>Date</th><th>Item</th><th class="num">Amount</th><th>Status</th></tr>
        </template>
        <tr>
          <td>2026-04-03</td><td>Booking.com payout</td><td class="num">€1,240.00</td>
          <td><UiBadge tone="success" dot>Mapped</UiBadge></td>
        </tr>
        <tr>
          <td>2026-04-05</td><td>Parking</td><td class="num">€30.00</td>
          <td><UiBadge tone="neutral" dot>Manual</UiBadge></td>
        </tr>
        <tr>
          <td>2026-04-07</td><td>Cleaning fee</td><td class="num">−€120.00</td>
          <td><UiBadge tone="warning" dot>Outgoing</UiBadge></td>
        </tr>
      </UiTable>
    </UiSection>

    <UiSection title="Banners & empty states">
      <div class="stack">
        <UiInlineBanner tone="info" title="Info">Reduced motion is respected.</UiInlineBanner>
        <UiInlineBanner tone="success" title="Success">Month opened and recurring entries synced.</UiInlineBanner>
        <UiInlineBanner tone="warning" title="Heads up">Some payouts are unmatched.</UiInlineBanner>
        <UiInlineBanner tone="danger" title="Error">Failed to reach the PMS backend.</UiInlineBanner>
        <UiEmptyState title="No data" description="There is nothing to show here yet.">
          <template #actions>
            <UiButton variant="primary">Create first</UiButton>
          </template>
        </UiEmptyState>
        <UiEmptyState
          illustration="inbox"
          title="Nothing here yet"
          description="Default empty-list illustration for tables and widgets."
        />
        <UiEmptyState
          illustration="no-results"
          title="No results"
          description="Shown when a filter or search excludes every record."
        />
        <UiEmptyState
          illustration="error"
          title="Something went wrong"
          description="Retry variant for failed loads."
        >
          <template #actions>
            <UiButton variant="secondary">Retry</UiButton>
          </template>
        </UiEmptyState>
      </div>
    </UiSection>

    <UiSection title="Loading placeholders">
      <UiCard>
        <div class="stack">
          <UiSkeleton variant="text" />
          <UiSkeleton variant="text" />
          <UiSkeleton variant="rect" />
          <div class="kpi-grid">
            <UiSkeleton variant="kpi" />
            <UiSkeleton variant="kpi" />
            <UiSkeleton variant="kpi" />
          </div>
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Toasts & dialogs">
      <UiCard>
        <div class="row">
          <UiButton variant="secondary" @click="toast.info('Informational toast')">Info toast</UiButton>
          <UiButton variant="primary" @click="toast.success('Everything saved')">Success toast</UiButton>
          <UiButton variant="ghost" @click="toast.warning('Double-check input')">Warning toast</UiButton>
          <UiButton variant="danger" @click="toast.error('Network unreachable')">Error toast</UiButton>
        </div>
        <div class="row">
          <UiButton variant="primary" @click="dialogOpen = true">Open dialog</UiButton>
          <UiButton variant="secondary" @click="persistentDialogOpen = true">Open persistent dialog</UiButton>
        </div>
      </UiCard>
    </UiSection>

    <UiDialog v-model:open="dialogOpen" title="Example dialog" size="sm">
      <p>Dialogs teleport to body, trap focus, and close on Esc or backdrop click.</p>
      <template #footer>
        <UiButton variant="secondary" size="sm" @click="dialogOpen = false">Cancel</UiButton>
        <UiButton variant="primary" size="sm" @click="dialogOpen = false">Confirm</UiButton>
      </template>
    </UiDialog>

    <UiDialog v-model:open="persistentDialogOpen" title="Persistent — explicit close only" size="md" persistent>
      <p>This variant ignores Esc and backdrop clicks. Use for pin reveals, deletion confirmations, etc.</p>
      <template #footer>
        <UiButton variant="primary" size="sm" @click="persistentDialogOpen = false">Got it</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
}
.row:last-child {
  margin-bottom: 0;
}
.form-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
}
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
}
.stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.spaced {
  margin-top: var(--space-3);
}
.num {
  font-variant-numeric: tabular-nums;
  text-align: right;
}
</style>
