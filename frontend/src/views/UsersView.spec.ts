import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

vi.mock('vue-router', () => ({
  RouterLink: { template: '<a><slot /></a>' },
}))

import { api } from '@/api/http'
import UsersView from './UsersView.vue'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

const sampleUsers = [
  { id: 1, email: 'alice@example.com', role: 'owner' },
  { id: 2, email: 'bob@example.com', role: 'super_admin' },
  { id: 3, email: 'carol@example.com', role: 'property_manager' },
  { id: 4, email: 'dan@example.com', role: 'read_only' },
]

describe('UsersView', () => {
  beforeEach(() => {
    apiMock.mockReset()
  })

  it('lists users returned by the API with role labels and tones', async () => {
    apiMock.mockResolvedValueOnce({ users: sampleUsers })
    const w = mount(UsersView)
    await flushPromises()
    const rows = w.findAll('tbody tr')
    expect(rows.length).toBe(4)
    expect(rows[0]?.text()).toContain('alice@example.com')
    expect(rows[0]?.text()).toContain('Owner')
    expect(rows[1]?.text()).toContain('Super Admin')
    expect(rows[2]?.text()).toContain('Property Manager')
    expect(rows[3]?.text()).toContain('Read Only')
  })

  it('shows the empty state once loaded with no users', async () => {
    apiMock.mockResolvedValueOnce({ users: [] })
    const w = mount(UsersView)
    await flushPromises()
    expect(w.text()).toContain('No users yet')
  })

  it('surfaces an error banner when the initial load fails', async () => {
    apiMock.mockRejectedValueOnce(new Error('forbidden'))
    const w = mount(UsersView)
    await flushPromises()
    expect(w.text()).toContain('forbidden')
  })

  it('creates a user, clears inputs and refreshes the list', async () => {
    apiMock
      .mockResolvedValueOnce({ users: [] }) // initial
      .mockResolvedValueOnce({}) // POST /api/users
      .mockResolvedValueOnce({ users: sampleUsers.slice(0, 1) }) // refresh
    const w = mount(UsersView)
    await flushPromises()
    const inputs = w.findAll('input')
    await inputs[0]?.setValue('new@example.com')
    await inputs[1]?.setValue('s3cret!!')
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(apiMock).toHaveBeenCalledWith(
      '/api/users',
      expect.objectContaining({
        method: 'POST',
        json: { email: 'new@example.com', password: 's3cret!!', role: 'owner' },
      }),
    )
    expect(w.text()).toContain('User created.')
    expect(w.text()).toContain('alice@example.com')
    // Inputs cleared after success.
    expect((inputs[0]?.element as HTMLInputElement).value).toBe('')
    expect((inputs[1]?.element as HTMLInputElement).value).toBe('')
  })

  it('shows a danger banner when user creation fails', async () => {
    apiMock
      .mockResolvedValueOnce({ users: [] })
      .mockRejectedValueOnce(new Error('Email already used'))
    const w = mount(UsersView)
    await flushPromises()
    await w.findAll('input')[0]?.setValue('dup@example.com')
    await w.findAll('input')[1]?.setValue('pw')
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(w.text()).toContain('Email already used')
  })
})
