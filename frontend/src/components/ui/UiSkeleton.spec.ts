import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiSkeleton from './UiSkeleton.vue'

describe('UiSkeleton', () => {
  it('renders default text variant count=1 and is hidden from AT', () => {
    const w = mount(UiSkeleton)
    const group = w.get('.ui-skeleton-group')
    expect(group.attributes('aria-hidden')).toBe('true')
    expect(w.findAll('.ui-skel--text').length).toBe(1)
  })

  it('renders N kpi skeletons when count is set', () => {
    const w = mount(UiSkeleton, { props: { variant: 'kpi', count: 4 } })
    expect(w.findAll('.ui-skel--kpi').length).toBe(4)
  })

  it('applies width/height on rect variant', () => {
    const w = mount(UiSkeleton, {
      props: { variant: 'rect', width: '50%', height: '80px' },
    })
    const el = w.get('.ui-skel--rect')
    expect(el.attributes('style')).toContain('width: 50%')
    expect(el.attributes('style')).toContain('height: 80px')
  })

  it('renders row variant', () => {
    const w = mount(UiSkeleton, { props: { variant: 'row', count: 3 } })
    expect(w.findAll('.ui-skel--row').length).toBe(3)
  })
})
