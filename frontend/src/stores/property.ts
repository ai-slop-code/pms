import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { api } from '@/api/http'

export interface Property {
  id: number
  name: string
  timezone: string
  default_language: string
  default_currency?: string
  invoice_code?: string | null
  owner_user_id: number
  week_starts_on?: 'monday' | 'sunday'
  active: boolean
}

const STORAGE_KEY = 'pms_current_property_id'

export const usePropertyStore = defineStore('property', () => {
  const list = ref<Property[]>([])
  const currentId = ref<number | null>(null)

  function loadStored() {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v) {
      const n = parseInt(v, 10)
      if (!Number.isNaN(n)) currentId.value = n
    }
  }

  async function fetchList() {
    const r = await api<{ properties: Property[] }>('/api/properties')
    list.value = r.properties
    if (currentId.value && !list.value.some((p) => p.id === currentId.value)) {
      currentId.value = list.value[0]?.id ?? null
    }
    if (!currentId.value && list.value.length) {
      currentId.value = list.value[0]?.id ?? null
    }
  }

  watch(currentId, (id) => {
    if (id) localStorage.setItem(STORAGE_KEY, String(id))
    else localStorage.removeItem(STORAGE_KEY)
  })

  return { list, currentId, loadStored, fetchList }
})
