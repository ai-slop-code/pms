<script setup lang="ts">
import UiSection from '@/components/ui/UiSection.vue'
import UiTable from '@/components/ui/UiTable.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { fmt } from './format'
import type { Invoice } from '@/api/types/invoice'

defineProps<{
  files: NonNullable<Invoice['files']>
}>()
</script>

<template>
  <UiSection title="Version history">
    <UiTable>
      <template #head>
        <tr>
          <th>Version</th>
          <th>Created</th>
          <th class="num">Size</th>
          <th>Path</th>
        </tr>
      </template>
      <tr v-for="file in files" :key="file.id">
        <td><UiBadge tone="neutral">v{{ file.version }}</UiBadge></td>
        <td>{{ fmt(file.created_at) }}</td>
        <td class="num">{{ (file.file_size_bytes / 1024).toFixed(1) }} KB</td>
        <td class="muted">{{ file.file_path }}</td>
      </tr>
    </UiTable>
  </UiSection>
</template>

<style scoped>
.muted {
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
