<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Tooltip,
  Filler,
  type ChartDataset,
  type ChartOptions,
} from 'chart.js'

// Tree-shaken registration: only what this line chart needs.
Chart.register(
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Tooltip,
  Filler,
)

export interface UiLineChartSeries {
  /** Human-readable legend label. */
  label: string
  /** Numeric values, one per x-axis label. Nulls render a gap. */
  data: Array<number | null>
  /** Optional override; defaults to --viz-{N} token for series index. */
  color?: string
  /** Optional secondary axis id — e.g. 'adr' for a dual-axis chart. */
  yAxisId?: string
  /** Optional formatter for tooltip values (e.g. currency, percent). */
  formatValue?: (v: number) => string
}

export interface UiLineChartAxis {
  id: string
  position?: 'left' | 'right'
  /** Formatter for tick labels. */
  tickFormat?: (v: number) => string
  /** Optional min / max clamp; otherwise auto-scale. */
  min?: number
  max?: number
}

interface Props {
  labels: string[]
  series: UiLineChartSeries[]
  /** Required descriptive label for screen readers. */
  ariaLabel: string
  /** Optional second axis. If omitted, all series share the left axis. */
  axes?: UiLineChartAxis[]
  /** Fixed chart height in px (default 180, matching hand-rolled chart).
   *  Must be a definite size so Chart.js with maintainAspectRatio:false
   *  doesn't grow the canvas on each ResizeObserver tick. */
  height?: number
}

const props = withDefaults(defineProps<Props>(), {
  axes: () => [],
  height: 180,
})

const canvas = ref<HTMLCanvasElement | null>(null)
const root = ref<HTMLElement | null>(null)
let chart: Chart | null = null

function resolveToken(name: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

function seriesColor(idx: number, override?: string): string {
  if (override) return override
  // --viz-1..--viz-8 defined in tokens.css; this array MUST mirror the token
  // hex values exactly — it's only used when the CSS variable lookup fails
  // (e.g. during SSR or in a test environment without a stylesheet).
  const fallbacks = [
    '#2563eb', // --viz-1
    '#059669', // --viz-2
    '#d97706', // --viz-3
    '#7c3aed', // --viz-4
    '#0891b2', // --viz-5
    '#db2777', // --viz-6
    '#b45309', // --viz-7
    '#475569', // --viz-8
  ]
  const token = `--viz-${(idx % 8) + 1}`
  return resolveToken(token, fallbacks[idx % fallbacks.length] ?? '#2563eb')
}

const prefersReducedMotion = computed(() => {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
})

function buildDatasets(): ChartDataset<'line'>[] {
  return props.series.map((s, i) => {
    const color = seriesColor(i, s.color)
    return {
      label: s.label,
      data: s.data,
      borderColor: color,
      backgroundColor: color,
      pointBackgroundColor: color,
      pointBorderColor: color,
      borderWidth: 2,
      tension: 0.2,
      pointRadius: 2,
      pointHoverRadius: 4,
      spanGaps: false,
      yAxisID: s.yAxisId ?? 'y',
    }
  })
}

function buildOptions(): ChartOptions<'line'> {
  const textMuted = resolveToken('--color-text-muted', '#64748b')
  const border = resolveToken('--color-border', '#e2e8f0')

  const scales: ChartOptions<'line'>['scales'] = {
    x: {
      ticks: { color: textMuted, font: { size: 11 } },
      grid: { color: border, display: false },
    },
    y: {
      beginAtZero: true,
      ticks: { color: textMuted, font: { size: 11 } },
      grid: { color: border },
    },
  }

  for (const axis of props.axes) {
    scales[axis.id] = {
      type: 'linear',
      position: axis.position ?? 'left',
      beginAtZero: true,
      min: axis.min,
      max: axis.max,
      ticks: {
        color: textMuted,
        font: { size: 11 },
        callback(value) {
          const n = typeof value === 'number' ? value : Number(value)
          return axis.tickFormat ? axis.tickFormat(n) : String(value)
        },
      },
      grid: { color: border, drawOnChartArea: axis.id === 'y' },
    }
  }

  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: prefersReducedMotion.value ? false : { duration: 250 },
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        enabled: true,
        backgroundColor: resolveToken('--color-surface', '#ffffff'),
        titleColor: resolveToken('--color-text', '#0f172a'),
        bodyColor: resolveToken('--color-text', '#0f172a'),
        borderColor: border,
        borderWidth: 1,
        padding: 8,
        callbacks: {
          label(ctx) {
            const s = props.series[ctx.datasetIndex]
            const raw = ctx.parsed.y
            if (!s) return ''
            if (raw == null || Number.isNaN(raw)) return `${s.label}: —`
            return `${s.label}: ${s.formatValue ? s.formatValue(raw) : raw}`
          },
        },
      },
    },
    scales,
  }
}

function mountChart() {
  if (!canvas.value) return
  destroyChart()
  chart = new Chart(canvas.value, {
    type: 'line',
    data: {
      labels: props.labels,
      datasets: buildDatasets(),
    },
    options: buildOptions(),
  })
}

function destroyChart() {
  if (chart) {
    chart.destroy()
    chart = null
  }
}

onMounted(() => {
  mountChart()
})

watch(
  () => [props.labels, props.series, props.axes],
  () => {
    if (!chart) {
      mountChart()
      return
    }
    chart.data.labels = props.labels
    chart.data.datasets = buildDatasets()
    chart.options = buildOptions()
    chart.update()
  },
  { deep: true },
)

onBeforeUnmount(destroyChart)

// Accessible data-table fallback: rendered visually hidden but available to
// screen readers and when Canvas is unavailable (e.g. print).
const hasAnyData = computed(() =>
  props.series.some((s) => s.data.some((v) => v != null && !Number.isNaN(v))),
)
</script>

<template>
  <figure
    ref="root"
    class="ui-line-chart"
    :style="{ height: `${height}px` }"
  >
    <canvas
      ref="canvas"
      class="ui-line-chart__canvas"
      role="img"
      :aria-label="ariaLabel"
    />
    <figcaption class="ui-line-chart__sr-fallback sr-only">
      <span>{{ ariaLabel }}</span>
      <table v-if="hasAnyData">
        <caption>{{ ariaLabel }}</caption>
        <thead>
          <tr>
            <th scope="col">Label</th>
            <th v-for="s in series" :key="s.label" scope="col">{{ s.label }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(label, i) in labels" :key="label">
            <th scope="row">{{ label }}</th>
            <td v-for="s in series" :key="s.label">
              <template v-if="s.data[i] == null">—</template>
              <template v-else>
                {{ s.formatValue ? s.formatValue(s.data[i] as number) : s.data[i] }}
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </figcaption>
  </figure>
</template>

<style scoped>
.ui-line-chart {
  position: relative;
  margin: 0;
  width: 100%;
}
.ui-line-chart__canvas {
  position: absolute;
  inset: 0;
  display: block;
}
</style>
