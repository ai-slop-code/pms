<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatShortDateTime, isoTitle } from '@/utils/format'
import type { NukiKeypadCode as KeypadCode } from '@/api/types/nuki'

defineProps<{
  codes: KeypadCode[]
  revealingCodeId: number | null
}>()

const emit = defineEmits<{
  reveal: [code: KeypadCode]
  delete: [externalId: string]
}>()

const fmt = formatShortDateTime
</script>

<template>
  <UiSection title="Enabled access codes">
    <UiTable
      :empty="!codes.length"
      empty-text="No enabled access codes found. Run Nuki access sync first."
    >
      <template #head>
        <tr>
          <th>Name</th>
          <th>PIN</th>
          <th>Window</th>
          <th>Source</th>
          <th>External ID</th>
          <th aria-label="Actions" />
        </tr>
      </template>
      <tr v-for="c in codes" :key="c.id">
        <td>{{ c.name || '—' }}</td>
        <td><code class="pin">{{ c.access_code_masked || '—' }}</code></td>
        <td>
          <time :datetime="c.valid_from || undefined" :title="isoTitle(c.valid_from)">{{ fmt(c.valid_from) }}</time>
          →
          <time :datetime="c.valid_until || undefined" :title="isoTitle(c.valid_until)">{{ fmt(c.valid_until) }}</time>
        </td>
        <td>
          <UiBadge :tone="c.pms_linked ? 'info' : 'neutral'">
            {{ c.pms_linked ? 'PMS' : 'External' }}
          </UiBadge>
        </td>
        <td class="muted">{{ c.external_nuki_id }}</td>
        <td class="row-actions">
          <UiButton
            variant="ghost"
            size="sm"
            :loading="revealingCodeId === c.id"
            @click="emit('reveal', c)"
          >Reveal</UiButton>
          <UiButton
            v-if="c.pms_linked"
            variant="ghost"
            size="sm"
            @click="emit('delete', c.external_nuki_id)"
          >Delete</UiButton>
          <span v-else class="muted">Read-only</span>
        </td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
.row-actions {
  text-align: right;
  white-space: nowrap;
}
.pin {
  font-family: var(--font-family-mono, monospace);
  font-size: var(--font-size-sm);
  padding: 2px 6px;
  background: var(--color-sunken);
  border-radius: var(--radius-sm);
  margin-right: var(--space-2);
}
</style>
