import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FinanceOverviewTab from './FinanceOverviewTab.vue'
import type { FinanceLongTermComparison, FinanceSummary } from '@/api/types/finance'
import { formatEuros } from '@/utils/format'

const summary: FinanceSummary = {
  month: '2026-04',
  total_incoming_cents: 0,
  total_outgoing_cents: 0,
  monthly_incoming_cents: 0,
  monthly_outgoing_cents: 0,
  monthly_net_cents: 0,
  property_income_cents: 0,
  monthly_property_income_cents: 0,
  cleaner_expense_cents: 0,
  cleaner_margin: 0,
  breakdown: [],
  generated_entry_sync: { status: 'synced' },
}

function comparison(longTermNetCents: number | null, outcome: FinanceLongTermComparison['outcome'] = 'behind'): FinanceLongTermComparison {
  return {
    status: 'configured',
    rate_id: 1,
    effective_from_month: '2026-01',
    monthly_rent_cents: 90000,
    eligible_outgoing_cents: longTermNetCents === null ? null : 57272,
    long_term_net_cents: longTermNetCents,
    short_term_difference_cents: 0,
    outcome,
  }
}

function mountOverview(
  longTermNetCents: number | null,
  recognizedNetCents: number | null,
  outcome: FinanceLongTermComparison['outcome'] = 'behind',
) {
  return mount(FinanceOverviewTab, {
    props: {
      summary,
      recognizedGrossCents: 0,
      recognizedNetCents,
      recognizedNetAvailable: recognizedNetCents !== null,
      longTermComparison: comparison(longTermNetCents, outcome),
      month: '2026-04',
      canManageRent: false,
    },
  })
}

function longTermCard(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('.ui-kpi').find((card) => card.text().includes('Long-term net'))!
}

describe('FinanceOverviewTab long-term net card', () => {
  it.each([
    { recognizedNetCents: -30135, valueCents: -32728, tone: 'danger' },
    { recognizedNetCents: 0, valueCents: -32728, tone: 'danger' },
    { recognizedNetCents: 32727, valueCents: -1, tone: 'danger' },
    { recognizedNetCents: 32728, valueCents: 0, tone: 'default' },
    { recognizedNetCents: 32729, valueCents: 1, tone: 'success' },
    { recognizedNetCents: 40000, valueCents: 7272, tone: 'success' },
  ])('derives the main figure and tone from recognized net', ({ recognizedNetCents, valueCents, tone }) => {
    const wrapper = mountOverview(32728, recognizedNetCents)
    const card = longTermCard(wrapper)

    expect(card.text()).toContain(formatEuros(valueCents))
    expect(card.classes()).toContain(`ui-kpi--tone-${tone}`)
    expect(card.text()).toContain(`Long term rent: ${formatEuros(32728)}`)
  })

  it('uses the displayed value sign instead of the legacy outcome', () => {
    const wrapper = mountOverview(-30000, -10000, 'ahead')
    const card = longTermCard(wrapper)

    expect(card.text()).toContain(formatEuros(-30000))
    expect(card.classes()).toContain('ui-kpi--tone-danger')
    expect(card.text()).toContain(`Long term rent: ${formatEuros(-30000)}`)
    expect(card.text()).not.toContain('Short-term ahead')
  })

  it('supports expenses exceeding rent without letting negative recognized net lower the baseline', () => {
    const wrapper = mountOverview(-30000, -10000)
    const card = longTermCard(wrapper)

    expect(card.text()).toContain(formatEuros(-30000))
    expect(card.text()).toContain(`Long term rent: ${formatEuros(-30000)}`)
  })

  it('recalculates the main figure and tone when the recognized net changes', async () => {
    const wrapper = mountOverview(32728, 0)
    const card = longTermCard(wrapper)
    const comparisonProp = wrapper.props('longTermComparison') as FinanceLongTermComparison

    await wrapper.setProps({ recognizedNetCents: 40000, recognizedNetAvailable: true })

    expect(card.text()).toContain(formatEuros(7272))
    expect(card.classes()).toContain('ui-kpi--tone-success')
    expect(card.text()).toContain(`Long term rent: ${formatEuros(comparisonProp.long_term_net_cents)}`)
  })

  it('does not render a numerical result when recognition is unavailable', () => {
    const wrapper = mountOverview(32728, null)
    const card = longTermCard(wrapper)

    expect(card.text()).toContain('Unavailable')
    expect(card.text()).toContain('Not available for the current selection')
    expect(card.text()).not.toContain(formatEuros(-32728))
    expect(card.text()).not.toContain('Long term rent:')
  })

  it('keeps the existing unconfigured state', () => {
    const wrapper = mount(FinanceOverviewTab, {
      props: {
        summary,
        recognizedGrossCents: 0,
        recognizedNetCents: 0,
        recognizedNetAvailable: true,
        longTermComparison: {
          status: 'not_configured',
          rate_id: null,
          effective_from_month: null,
          monthly_rent_cents: null,
          eligible_outgoing_cents: null,
          long_term_net_cents: null,
          short_term_difference_cents: null,
          outcome: null,
        },
        month: '2026-04',
        canManageRent: false,
      },
    })
    const card = longTermCard(wrapper)

    expect(card.text()).toContain('Long-term rent not configured')
    expect(card.text()).toContain('Configure a monthly benchmark rent to compare results.')
    expect(card.text()).not.toContain('Long term rent:')
    expect(card.classes()).toContain('ui-kpi--tone-default')
  })

  it('removes the duplicate numerical explanation below the cards', () => {
    const wrapper = mountOverview(32728, 0)

    expect(wrapper.text()).not.toContain(`${formatEuros(90000)} rent`)
    expect(wrapper.text()).toContain('Monthly rent less recorded outgoing')
  })
})
