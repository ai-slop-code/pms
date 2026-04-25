import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import IllustrationEmptyInbox from './IllustrationEmptyInbox.vue'
import IllustrationNoResults from './IllustrationNoResults.vue'
import IllustrationError from './IllustrationError.vue'
import IllustrationKeys from './IllustrationKeys.vue'
import IllustrationDashboardWelcome from './IllustrationDashboardWelcome.vue'
import IllustrationInvoice from './IllustrationInvoice.vue'
import IllustrationSparkles from './IllustrationSparkles.vue'
import { illustrations } from './registry'

// The accessibility contract is shared across all illustrations per
// PMS_08 §12.2, so parameterise the assertions rather than duplicate them.
const cases = [
  ['EmptyInbox', IllustrationEmptyInbox],
  ['NoResults', IllustrationNoResults],
  ['Error', IllustrationError],
  ['Keys', IllustrationKeys],
  ['DashboardWelcome', IllustrationDashboardWelcome],
  ['Invoice', IllustrationInvoice],
  ['Sparkles', IllustrationSparkles],
] as const

describe('Illustrations', () => {
  for (const [name, Comp] of cases) {
    it(`${name}: decorative by default (aria-hidden, no role)`, () => {
      const w = mount(Comp)
      const svg = w.find('svg')
      expect(svg.exists()).toBe(true)
      expect(svg.attributes('aria-hidden')).toBe('true')
      expect(svg.attributes('role')).toBeUndefined()
      expect(svg.attributes('aria-label')).toBeUndefined()
    })

    it(`${name}: meaningful variant sets role=img + aria-label`, () => {
      const w = mount(Comp, { props: { ariaLabel: 'illustration' } })
      const svg = w.find('svg')
      expect(svg.attributes('role')).toBe('img')
      expect(svg.attributes('aria-label')).toBe('illustration')
      expect(svg.attributes('aria-hidden')).toBeUndefined()
    })
  }

  it('registry exposes the seven named illustrations', () => {
    expect(Object.keys(illustrations).sort()).toEqual(
      [
        'dashboard',
        'error',
        'inbox',
        'invoice',
        'keys',
        'no-results',
        'sparkles',
      ].sort(),
    )
  })
})
