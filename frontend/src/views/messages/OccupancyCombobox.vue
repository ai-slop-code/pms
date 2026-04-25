<script setup lang="ts">
import { computed, ref } from 'vue'
import { X } from 'lucide-vue-next'
import type { MessagesOccupancy as Occupancy } from '@/api/types/messages'
import { occLabel, fmtDate } from './helpers'

const props = defineProps<{
  occupancies: Occupancy[]
  selectedId: number | null
  disabled?: boolean
}>()

const emit = defineEmits<{ select: [occ: Occupancy]; clear: [] }>()

const search = ref('')
const open = ref(false)
const activeIndex = ref(-1)
const listboxId = 'msg-occ-listbox'
const inputId = 'msg-occ-combobox'

const filtered = computed(() => {
  const q = search.value.toLowerCase().trim()
  if (!q) return props.occupancies
  return props.occupancies.filter((o) => occLabel(o).toLowerCase().includes(q))
})

const activeOption = computed(() => {
  if (activeIndex.value < 0 || activeIndex.value >= filtered.value.length) return null
  return filtered.value[activeIndex.value] ?? null
})

const activeOptionId = computed(() => (activeOption.value ? `msg-occ-opt-${activeOption.value.id}` : ''))

const selectedLabel = computed(() => {
  if (props.selectedId == null) return ''
  const occ = props.occupancies.find((o) => o.id === props.selectedId)
  return occ ? occLabel(occ) : ''
})

function onFocus() {
  open.value = true
  search.value = ''
  activeIndex.value = -1
}

function onBlur() {
  open.value = false
  search.value = ''
  activeIndex.value = -1
}

function selectOption(occ: Occupancy) {
  emit('select', occ)
  open.value = false
  search.value = ''
  activeIndex.value = -1
}

function clearSelection() {
  emit('clear')
  search.value = ''
}

function onKey(event: KeyboardEvent) {
  const len = filtered.value.length
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      if (!open.value) open.value = true
      else if (activeIndex.value < len - 1) activeIndex.value += 1
      break
    case 'ArrowUp':
      event.preventDefault()
      if (activeIndex.value > -1) activeIndex.value -= 1
      break
    case 'Enter':
      event.preventDefault()
      if (activeOption.value) selectOption(activeOption.value)
      break
    case 'Escape':
      event.preventDefault()
      open.value = false
      search.value = ''
      activeIndex.value = -1
      break
    case 'Home':
      if (open.value && len > 0) {
        event.preventDefault()
        activeIndex.value = 0
      }
      break
    case 'End':
      if (open.value && len > 0) {
        event.preventDefault()
        activeIndex.value = len - 1
      }
      break
    default:
      if (event.key.length === 1 && /[a-zA-Z0-9]/.test(event.key)) {
        const q = event.key.toLowerCase()
        const idx = filtered.value.findIndex((o) => occLabel(o).toLowerCase().startsWith(q))
        if (idx >= 0) activeIndex.value = idx
      }
  }
}
</script>

<template>
  <div class="field">
    <label class="field__label" :for="inputId">Select a stay</label>
    <div class="occ-picker" :class="{ open }">
      <div class="occ-input-wrap">
        <input
          :id="inputId"
          class="occ-search"
          role="combobox"
          :aria-expanded="open"
          :aria-controls="listboxId"
          :aria-activedescendant="activeOptionId"
          aria-autocomplete="list"
          :placeholder="selectedLabel || (occupancies.length ? 'Search stays…' : 'No upcoming stays')"
          :value="open ? search : selectedLabel"
          :disabled="disabled || !occupancies.length"
          @focus="onFocus"
          @blur="onBlur"
          @input="search = ($event.target as HTMLInputElement).value"
          @keydown="onKey"
        />
        <button
          v-if="selectedId != null && !open"
          type="button"
          class="occ-clear"
          aria-label="Clear selection"
          @mousedown.prevent="clearSelection"
        ><X :size="14" aria-hidden="true" /></button>
      </div>
      <ul
        v-if="open && filtered.length"
        :id="listboxId"
        class="occ-list"
        role="listbox"
        aria-label="Upcoming stays"
      >
        <li
          v-for="(occ, idx) in filtered"
          :id="`msg-occ-opt-${occ.id}`"
          :key="occ.id"
          class="occ-option"
          role="option"
          :aria-selected="occ.id === selectedId"
          :class="{ active: idx === activeIndex, selected: occ.id === selectedId }"
          @mousedown.prevent="selectOption(occ)"
        >
          <span class="occ-name">{{ occ.guest_display_name || occ.raw_summary || `#${occ.id}` }}</span>
          <span class="occ-dates">{{ fmtDate(occ.start_at) }} → {{ fmtDate(occ.end_at) }}</span>
        </li>
      </ul>
      <div v-if="open && search && !filtered.length" class="occ-list occ-empty">
        No stays matching "{{ search }}"
      </div>
    </div>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.field__label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-muted);
}
.occ-picker { position: relative; }
.occ-input-wrap { position: relative; }
.occ-search {
  width: 100%;
  min-height: 36px;
  padding: 0 32px 0 var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font: var(--font-size-md) / 1.4 var(--font-family-sans);
  background: var(--color-surface);
  color: var(--color-text);
}
.occ-search:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: var(--focus-ring);
}
.occ-clear {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: var(--color-text-muted);
  padding: 4px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  min-height: 0;
}
.occ-clear:hover { color: var(--color-text); }
.occ-list {
  position: absolute;
  z-index: 50;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  max-height: 240px;
  overflow-y: auto;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-2);
  list-style: none;
  margin: 0;
  padding: 4px 0;
}
.occ-option {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
  gap: var(--space-3);
}
.occ-option:hover { background: var(--color-sunken); }
.occ-option.active {
  background: color-mix(in srgb, var(--color-primary) 15%, transparent);
  font-weight: 500;
}
.occ-option.selected {
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  font-weight: 500;
}
.occ-name {
  font-size: var(--font-size-sm);
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.occ-dates {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
  flex-shrink: 0;
}
.occ-empty {
  padding: var(--space-3);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  text-align: center;
}
</style>
