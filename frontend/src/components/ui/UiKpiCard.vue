<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { ArrowUpRight, ArrowDownRight, ArrowRight, Info } from 'lucide-vue-next'

interface Trend {
  direction: 'up' | 'down' | 'flat'
  label: string
  /** Optional override — if omitted direction controls colour with "up = success" default. */
  tone?: 'success' | 'warning' | 'danger' | 'neutral'
}

interface Props {
  label: string
  value: string | number
  tone?: 'default' | 'success' | 'warning' | 'danger'
  /** Larger 32px KPI figure (one per page max). */
  hero?: boolean
  trend?: Trend | null
  hint?: string
  calculationHelp?: string
}

const props = withDefaults(defineProps<Props>(), {
  tone: 'default',
  hero: false,
  trend: null,
  hint: '',
  calculationHelp: '',
})

const helpOpen = ref(false)
const helpHovered = ref(false)
const helpFocused = ref(false)
const helpId = `kpi-help-${useId()}`
const helpVisible = computed(() => helpOpen.value || helpHovered.value || helpFocused.value)

function dismissHelp() {
  helpOpen.value = false
  helpFocused.value = false
}

const trendTone = computed(() => {
  if (!props.trend) return 'neutral'
  if (props.trend.tone) return props.trend.tone
  if (props.trend.direction === 'up') return 'success'
  if (props.trend.direction === 'down') return 'danger'
  return 'neutral'
})

const trendIcon = computed(() => {
  if (!props.trend) return null
  if (props.trend.direction === 'up') return ArrowUpRight
  if (props.trend.direction === 'down') return ArrowDownRight
  return ArrowRight
})
</script>

<template>
  <article class="ui-kpi" :class="[`ui-kpi--tone-${tone}`, { 'ui-kpi--hero': hero }]">
    <div class="ui-kpi__label" @mouseenter="helpHovered = true" @mouseleave="helpHovered = false">
      <span>{{ label }}</span>
      <button
        v-if="calculationHelp"
        type="button"
        class="ui-kpi__help-trigger"
        :aria-label="`How ${label} is calculated`"
        :aria-controls="helpId"
        :aria-expanded="helpVisible"
        :aria-describedby="helpId"
        @click="helpOpen = !helpOpen"
        @focus="helpFocused = true"
        @blur="helpFocused = false"
        @keydown.esc="dismissHelp"
      >
        <Info :size="14" aria-hidden="true" />
      </button>
      <div
        v-if="calculationHelp && helpVisible"
        :id="helpId"
        class="ui-kpi__help"
        role="tooltip"
        @mouseenter="helpHovered = true"
        @mouseleave="helpHovered = false"
      >
        {{ calculationHelp }}
      </div>
    </div>
    <div class="ui-kpi__value num">{{ value }}</div>
    <div v-if="trend" class="ui-kpi__trend" :class="`ui-kpi__trend--${trendTone}`">
      <component :is="trendIcon" :size="14" aria-hidden="true" />
      <span>{{ trend.label }}</span>
    </div>
    <div v-if="hint" class="ui-kpi__hint">{{ hint }}</div>
  </article>
</template>

<style scoped>
.ui-kpi {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  min-height: 104px;
}

.ui-kpi__label {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.ui-kpi__help-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: var(--color-text-muted);
  background: transparent;
  cursor: pointer;
}
.ui-kpi__help-trigger:hover,
.ui-kpi__help-trigger:focus-visible {
  color: var(--color-text);
  background: var(--color-sunken);
  outline: 2px solid var(--color-focus);
  outline-offset: 1px;
}
.ui-kpi__help {
  position: absolute;
  z-index: 3;
  top: calc(100% + var(--space-2));
  left: 0;
  width: min(300px, calc(100vw - 2rem));
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  box-shadow: var(--shadow-2);
  color: var(--color-text);
  font-size: var(--font-size-xs);
  font-weight: 400;
  line-height: 1.4;
  text-transform: none;
  letter-spacing: normal;
}
.ui-kpi__value {
  font-size: var(--font-size-kpi);
  font-weight: 600;
  line-height: 1.1;
  color: var(--color-text);
  margin-top: var(--space-1);
}
.ui-kpi--hero .ui-kpi__value {
  font-size: var(--font-size-kpi-lg);
  font-weight: 700;
}

.ui-kpi--tone-success .ui-kpi__value {
  color: var(--success-fg);
}
.ui-kpi--tone-warning .ui-kpi__value {
  color: var(--warning-fg);
}
.ui-kpi--tone-danger .ui-kpi__value {
  color: var(--danger-fg);
}

.ui-kpi__trend {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--font-size-xs);
  font-weight: 500;
  margin-top: var(--space-1);
}
.ui-kpi__trend--success {
  color: var(--success-fg);
}
.ui-kpi__trend--warning {
  color: var(--warning-fg);
}
.ui-kpi__trend--danger {
  color: var(--danger-fg);
}
.ui-kpi__trend--neutral {
  color: var(--color-text-subtle);
}

.ui-kpi__hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-subtle);
}
</style>
