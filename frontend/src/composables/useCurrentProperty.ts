import { computed } from 'vue'
import { usePropertyStore, type Property } from '@/stores/property'

/**
 * Consolidates the `currentId` + `currentProperty` computeds every
 * property-scoped view was redeclaring. Returns the raw store too so
 * consumers that need `list` / `fetchList` don't have to call
 * `usePropertyStore()` a second time.
 *
 * Example:
 *
 * ```ts
 * const { pid, currentProperty } = useCurrentProperty()
 * watch(pid, () => { if (pid.value) load() })
 * ```
 */
export function useCurrentProperty() {
  const propertyStore = usePropertyStore()
  const pid = computed<number | null>(() => propertyStore.currentId)
  const currentProperty = computed<Property | null>(
    () => propertyStore.list.find((p) => p.id === propertyStore.currentId) ?? null,
  )
  return { pid, currentProperty, propertyStore }
}
