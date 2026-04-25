<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiCard from '@/components/ui/UiCard.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { displayStatus, statusTone } from './status'
import type { OccupancySyncRun as Run, OccupancyApiToken as TokenRow } from '@/api/types/occupancy'

defineProps<{
  source: { active: boolean; source_type: string } | null
  runs: Run[]
  tokens: TokenRow[]
  syncing: boolean
  newTokenPlain: string
  copiedExport: string
}>()

const emit = defineEmits<{
  toggleSource: []
  runSync: []
  createToken: []
  removeToken: [id: number]
  copyCurl: []
}>()
</script>

<template>
  <div>
    <UiSection
      title="ICS source"
      description="Set the Booking.com (or other) calendar URL under Properties → Profile & integrations. Toggle sync below."
    >
      <UiCard>
        <div v-if="source" class="source-row">
          <span>Source type: <code>{{ source.source_type }}</code></span>
          <UiBadge :tone="source.active ? 'success' : 'warning'" dot>
            {{ source.active ? 'Active' : 'Paused' }}
          </UiBadge>
          <UiButton variant="secondary" size="sm" @click="emit('toggleSource')">
            {{ source.active ? 'Pause sync' : 'Enable sync' }}
          </UiButton>
        </div>
        <div class="sync-actions">
          <UiButton variant="primary" :loading="syncing" @click="emit('runSync')">
            Run occupancy sync
          </UiButton>
        </div>
      </UiCard>
    </UiSection>

    <UiSection title="Sync history">
      <UiTable :empty="!runs.length" empty-text="No sync runs yet.">
        <template #head>
          <tr>
            <th>Started</th>
            <th>Status</th>
            <th class="num">Events</th>
            <th class="num">Upserted</th>
            <th>Trigger</th>
            <th>Error</th>
          </tr>
        </template>
        <tr v-for="r in runs" :key="r.id">
          <td>{{ r.started_at }}</td>
          <td>
            <UiBadge :tone="statusTone(r.status)" dot>{{ displayStatus(r.status) }}</UiBadge>
          </td>
          <td class="num">{{ r.events_seen }}</td>
          <td class="num">{{ r.occupancies_upserted }}</td>
          <td>{{ displayStatus(r.trigger) }}</td>
          <td class="error-cell">{{ r.error_message || '—' }}</td>
        </tr>
      </UiTable>
    </UiSection>

    <UiSection
      title="JSON export (n8n)"
      description="Create a token (shown once). Call the export endpoint with the token in an Authorization: Bearer header."
    >
      <UiCard>
        <p class="muted">
          <code>GET /api/properties/{id}/occupancy-export</code> — pass the token via
          <code>Authorization: Bearer …</code> or <code>X-Export-Token</code>. Query string
          fallback (<code>?token=…</code>) is deprecated.
        </p>
        <div class="sync-actions">
          <UiButton variant="primary" @click="emit('createToken')">Create export token</UiButton>
        </div>
        <div v-if="newTokenPlain" class="token-callout">
          <strong>Save this token now:</strong>
          <code class="token-callout__value">{{ newTokenPlain }}</code>
          <div class="token-callout__actions">
            <UiButton variant="secondary" size="sm" @click="emit('copyCurl')">Copy curl command</UiButton>
            <span v-if="copiedExport" class="token-callout__hint">{{ copiedExport }}</span>
          </div>
        </div>
      </UiCard>

      <UiTable :empty="!tokens.length" empty-text="No export tokens yet.">
        <template #head>
          <tr>
            <th class="num">ID</th>
            <th>Created</th>
            <th>Last used</th>
            <th aria-label="Actions" />
          </tr>
        </template>
        <tr v-for="t in tokens" :key="t.id">
          <td class="num">{{ t.id }}</td>
          <td>{{ t.created_at }}</td>
          <td>{{ t.last_used_at || '—' }}</td>
          <td class="row-actions">
            <UiButton variant="ghost" size="sm" @click="emit('removeToken', t.id)">Revoke</UiButton>
          </td>
        </tr>
      </UiTable>
    </UiSection>
  </div>
</template>

<style scoped>
.source-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
  margin-bottom: var(--space-3);
}
.sync-actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-2);
}
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  margin: 0 0 var(--space-3);
}
.error-cell {
  font-size: var(--font-size-xs);
  color: var(--danger-fg);
}
.token-callout {
  margin-top: var(--space-4);
  padding: var(--space-3) var(--space-4);
  background: var(--color-sunken);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.token-callout__value {
  word-break: break-all;
  font-family: var(--font-family-mono, monospace);
  background: var(--color-surface);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  border: 1px solid var(--color-border);
}
.token-callout__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.token-callout__hint {
  font-size: var(--font-size-xs);
  color: var(--success-fg);
}
.row-actions {
  text-align: right;
}
</style>
