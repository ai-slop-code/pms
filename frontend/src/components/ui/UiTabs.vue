<script setup lang="ts">
import { computed } from 'vue'

interface Tab {
  id: string
  label: string
  disabled?: boolean
}

interface Props {
  modelValue: string
  tabs: Tab[]
  ariaLabel?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{ (e: 'update:modelValue', id: string): void }>()

const activeId = computed(() => props.modelValue)

function select(id: string) {
  if (id === activeId.value) return
  emit('update:modelValue', id)
}

function onKey(e: KeyboardEvent, index: number) {
  const enabled = props.tabs.filter((t) => !t.disabled)
  if (!enabled.length) return
  const currentTab = props.tabs[index]
  if (!currentTab) return
  const currentIdx = enabled.findIndex((t) => t.id === currentTab.id)
  let nextIdx = currentIdx
  if (e.key === 'ArrowRight') nextIdx = (currentIdx + 1) % enabled.length
  else if (e.key === 'ArrowLeft') nextIdx = (currentIdx - 1 + enabled.length) % enabled.length
  else if (e.key === 'Home') nextIdx = 0
  else if (e.key === 'End') nextIdx = enabled.length - 1
  else return
  e.preventDefault()
  const nextTab = enabled[nextIdx]
  if (!nextTab) return
  select(nextTab.id)
  const el = document.getElementById(`tab-${nextTab.id}`)
  el?.focus()
}
</script>

<template>
  <div class="ui-tabs">
    <div role="tablist" :aria-label="ariaLabel" class="ui-tabs__list">
      <button
        v-for="(tab, idx) in tabs"
        :id="`tab-${tab.id}`"
        :key="tab.id"
        role="tab"
        class="ui-tabs__tab"
        :class="{ 'ui-tabs__tab--active': tab.id === activeId }"
        :aria-selected="tab.id === activeId"
        :aria-controls="`panel-${tab.id}`"
        :tabindex="tab.id === activeId ? 0 : -1"
        :disabled="tab.disabled || undefined"
        type="button"
        @click="select(tab.id)"
        @keydown="onKey($event, idx)"
      >
        {{ tab.label }}
      </button>
    </div>
    <div
      :id="`panel-${activeId}`"
      role="tabpanel"
      :aria-labelledby="`tab-${activeId}`"
      class="ui-tabs__panel"
    >
      <slot :active="activeId" />
    </div>
  </div>
</template>

<style scoped>
.ui-tabs__list {
  display: flex;
  gap: var(--space-1);
  border-bottom: 1px solid var(--color-border);
  overflow-x: auto;
  scrollbar-width: none;
}
.ui-tabs__list::-webkit-scrollbar {
  display: none;
}
.ui-tabs__tab {
  appearance: none;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font: 500 var(--font-size-sm) / 1 var(--font-family-sans);
  padding: var(--space-3) var(--space-4);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  cursor: pointer;
  white-space: nowrap;
  transition: color var(--motion-1) var(--ease-standard),
    border-color var(--motion-1) var(--ease-standard);
}
.ui-tabs__tab:hover:not(:disabled):not(.ui-tabs__tab--active) {
  color: var(--color-text);
}
.ui-tabs__tab--active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}
.ui-tabs__tab:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ui-tabs__panel {
  padding-top: var(--space-4);
}
</style>
