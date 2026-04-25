import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

import { api } from '@/api/http'
import { usePropertyStore, type Property } from './property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

/**
 * jsdom 25 without persistent-storage config exposes `localStorage` as a
 * getter that throws on mutation, so the specs install a tiny in-memory
 * implementation before each run.
 */
function installLocalStorageStub() {
  const store = new Map<string, string>()
  const stub: Storage = {
    get length() {
      return store.size
    },
    clear: () => store.clear(),
    getItem: (k: string) => (store.has(k) ? (store.get(k) as string) : null),
    key: (i: number) => Array.from(store.keys())[i] ?? null,
    removeItem: (k: string) => {
      store.delete(k)
    },
    setItem: (k: string, v: string) => {
      store.set(k, v)
    },
  }
  Object.defineProperty(globalThis, 'localStorage', {
    value: stub,
    configurable: true,
    writable: true,
  })
}

function makeProperty(overrides: Partial<Property> = {}): Property {
  return {
    id: 1,
    name: 'Sample',
    timezone: 'Europe/Bratislava',
    default_language: 'en',
    owner_user_id: 1,
    active: true,
    ...overrides,
  }
}

describe('property store', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  describe('loadStored', () => {
    it('restores a previously persisted property id', () => {
      localStorage.setItem('pms_current_property_id', '7')
      const store = usePropertyStore()
      store.loadStored()
      expect(store.currentId).toBe(7)
    })

    it('ignores a non-numeric stored value', () => {
      localStorage.setItem('pms_current_property_id', 'not-a-number')
      const store = usePropertyStore()
      store.loadStored()
      expect(store.currentId).toBeNull()
    })

    it('leaves currentId null when nothing is stored', () => {
      const store = usePropertyStore()
      store.loadStored()
      expect(store.currentId).toBeNull()
    })
  })

  describe('fetchList', () => {
    it('populates the list and auto-selects the first property', async () => {
      apiMock.mockResolvedValueOnce({
        properties: [makeProperty({ id: 1 }), makeProperty({ id: 2 })],
      })
      const store = usePropertyStore()
      await store.fetchList()
      expect(store.list.map((p) => p.id)).toEqual([1, 2])
      expect(store.currentId).toBe(1)
    })

    it('preserves the current selection when it still exists in the list', async () => {
      apiMock.mockResolvedValueOnce({
        properties: [makeProperty({ id: 1 }), makeProperty({ id: 2 })],
      })
      const store = usePropertyStore()
      store.currentId = 2
      await store.fetchList()
      expect(store.currentId).toBe(2)
    })

    it('reassigns to the first available when the current property disappears', async () => {
      apiMock.mockResolvedValueOnce({
        properties: [makeProperty({ id: 10 }), makeProperty({ id: 11 })],
      })
      const store = usePropertyStore()
      store.currentId = 99
      await store.fetchList()
      expect(store.currentId).toBe(10)
    })

    it('clears currentId when the list comes back empty', async () => {
      apiMock.mockResolvedValueOnce({ properties: [] })
      const store = usePropertyStore()
      store.currentId = 42
      await store.fetchList()
      expect(store.currentId).toBeNull()
    })
  })

  describe('currentId localStorage sync', () => {
    it('writes to localStorage when the id changes', async () => {
      const store = usePropertyStore()
      store.currentId = 5
      // The watcher runs on the next microtask.
      await Promise.resolve()
      expect(localStorage.getItem('pms_current_property_id')).toBe('5')
    })

    it('removes the entry when currentId is cleared', async () => {
      localStorage.setItem('pms_current_property_id', '3')
      const store = usePropertyStore()
      store.currentId = 3
      await Promise.resolve()
      store.currentId = null
      await Promise.resolve()
      expect(localStorage.getItem('pms_current_property_id')).toBeNull()
    })
  })
})
