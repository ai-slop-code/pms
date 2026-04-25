<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { Plus } from 'lucide-vue-next'
import { usePropertyStore } from '@/stores/property'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiTable from '@/components/ui/UiTable.vue'

const props = usePropertyStore()
const router = useRouter()

onMounted(() => {
  props.fetchList().catch(() => {})
})

function goNew() {
  router.push('/properties/new')
}
</script>

<template>
  <div>
    <UiPageHeader title="Properties" lede="All properties you can manage.">
      <template #actions>
        <UiButton variant="primary" @click="goNew">
          <template #iconLeft><Plus :size="16" aria-hidden="true" /></template>
          New property
        </UiButton>
      </template>
    </UiPageHeader>

    <UiTable :empty="!props.list.length" empty-text="No properties found yet.">
      <template #head>
        <tr>
          <th>Name</th>
          <th>Timezone</th>
          <th aria-label="Actions" />
        </tr>
      </template>
      <tr v-for="p in props.list" :key="p.id">
        <td>{{ p.name }}</td>
        <td>{{ p.timezone }}</td>
        <td>
          <RouterLink :to="`/properties/${p.id}`" class="properties-link">
            Open details
          </RouterLink>
        </td>
      </tr>
    </UiTable>
  </div>
</template>

<style scoped>
.properties-link {
  color: var(--color-primary);
  font-weight: 500;
}
</style>
