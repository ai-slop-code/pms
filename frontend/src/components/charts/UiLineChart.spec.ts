import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import UiLineChart from './UiLineChart.vue'

// Chart.js is canvas-based and heavy; mock it so unit tests don't need a real
// 2d context. We only assert the wrapper's contract: a11y fallback, reactive
// updates, tear-down. Library behaviour is exercised manually / in e2e.
const chartInstances: Array<{ destroy: ReturnType<typeof vi.fn>; update: ReturnType<typeof vi.fn> }> = []
vi.mock('chart.js', () => {
  class Chart {
    data: unknown
    options: unknown
    destroy = vi.fn()
    update = vi.fn()
    constructor(_el: unknown, config: { data: unknown; options: unknown }) {
      this.data = config.data
      this.options = config.options
      chartInstances.push(this as unknown as { destroy: ReturnType<typeof vi.fn>; update: ReturnType<typeof vi.fn> })
    }
    static register = vi.fn()
  }
  return {
    Chart,
    LineController: {},
    LineElement: {},
    PointElement: {},
    LinearScale: {},
    CategoryScale: {},
    Tooltip: {},
    Filler: {},
  }
})

describe('UiLineChart', () => {
  beforeEach(() => {
    chartInstances.length = 0
  })

  it('applies the aria-label to the canvas', () => {
    const w = mount(UiLineChart, {
      props: {
        ariaLabel: 'Monthly revenue line chart',
        labels: ['Jan', 'Feb', 'Mar'],
        series: [{ label: 'Revenue', data: [10, 20, 30] }],
      },
    })
    const canvas = w.get('canvas')
    expect(canvas.attributes('role')).toBe('img')
    expect(canvas.attributes('aria-label')).toBe('Monthly revenue line chart')
  })

  it('renders a screen-reader-only data table fallback', () => {
    const w = mount(UiLineChart, {
      props: {
        ariaLabel: 'Trend',
        labels: ['Jan', 'Feb'],
        series: [
          { label: 'A', data: [1, 2] },
          { label: 'B', data: [null, 5] },
        ],
      },
    })
    const table = w.get('table')
    // Two series + label column → 3 headers.
    expect(table.findAll('thead th')).toHaveLength(3)
    // Two data rows for two labels.
    expect(table.findAll('tbody tr')).toHaveLength(2)
    // Null is rendered as em dash in the table fallback.
    expect(table.text()).toContain('—')
  })

  it('omits the data table when there is no data at all', () => {
    const w = mount(UiLineChart, {
      props: {
        ariaLabel: 'Trend',
        labels: ['Jan'],
        series: [{ label: 'A', data: [null] }],
      },
    })
    expect(w.find('table').exists()).toBe(false)
  })

  it('formats values via formatValue in the fallback table', () => {
    const w = mount(UiLineChart, {
      props: {
        ariaLabel: 'x',
        labels: ['Jan'],
        series: [
          {
            label: 'Revenue',
            data: [1234],
            formatValue: (v: number) => `€${v}`,
          },
        ],
      },
    })
    expect(w.get('tbody').text()).toContain('€1234')
  })

  it('instantiates a Chart.js line chart with the provided labels & datasets', async () => {
    mount(UiLineChart, {
      props: {
        ariaLabel: 'x',
        labels: ['a', 'b'],
        series: [{ label: 'S', data: [1, 2] }],
      },
      attachTo: document.body,
    })
    await flushPromises()
    expect(chartInstances).toHaveLength(1)
    const cfg = chartInstances[0] as unknown as { data: { labels: unknown; datasets: unknown[] } }
    expect(cfg.data.labels).toEqual(['a', 'b'])
    expect(cfg.data.datasets).toHaveLength(1)
  })

  it('destroys the chart on unmount', async () => {
    const w = mount(UiLineChart, {
      props: {
        ariaLabel: 'x',
        labels: ['a'],
        series: [{ label: 'S', data: [1] }],
      },
      attachTo: document.body,
    })
    await flushPromises()
    const instance = chartInstances[0]
    w.unmount()
    expect(instance?.destroy).toHaveBeenCalledTimes(1)
  })
})
