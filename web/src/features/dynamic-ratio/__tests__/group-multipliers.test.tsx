/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  GroupMultiplierCard,
  GroupMultiplierPanel,
} from '../group-multiplier-panel'
import {
  formatMultiplier,
  groupPolicySchema,
  type GroupMultiplierStatus,
} from '../group-policy'
import { GroupPolicyDialog } from '../group-policy-dialog'

const status: GroupMultiplierStatus = {
  group: 'Coding',
  description: 'Coding tools',
  version: 'version-1',
  policy: {
    mode: 'concurrency',
    tiers: [
      { minimum: 0, multiplier: 1 },
      { minimum: 8, multiplier: 2 },
      { minimum: 15, multiplier: 3 },
    ],
  },
  base_ratio: 2,
  concurrency: 7,
  factor: 1,
  effective_ratio: 2,
  next_request_ratio: 4,
  next_tier: { minimum: 8, multiplier: 2 },
}
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.getState().auth.setUser({ id: 42, username: 'alice', role: 1 })
})
afterEach(async () => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
  await i18next.changeLanguage('en')
})

describe('group multiplier pricing', () => {
  it('shows the next request price at a boundary without highlighting the next tier early', () => {
    render(<GroupMultiplierCard status={status} />)
    expect(
      screen.getByText('Current group multiplier').parentElement
    ).toHaveTextContent('×2')
    expect(screen.getByText(/Next request estimate/)).toHaveTextContent('×4')
    const list = screen.getByRole('list', { name: 'Concurrency tiers' })
    const active = within(list)
      .getAllByRole('listitem')
      .filter((item) => item.getAttribute('aria-current') === 'step')
    expect(active).toHaveLength(1)
    expect(active[0]).toHaveTextContent('≥0')
  })
  it('does not show an active tier or estimated price when load is unavailable', () => {
    render(
      <GroupMultiplierCard
        status={{
          ...status,
          concurrency: null,
          factor: null,
          effective_ratio: null,
          next_request_ratio: null,
          next_tier: null,
        }}
      />
    )
    expect(screen.getByText('Unavailable')).toBeInTheDocument()
    expect(screen.queryByText(/Next request estimate/)).not.toBeInTheDocument()
    expect(
      screen
        .getAllByRole('listitem')
        .every((item) => !item.hasAttribute('aria-current'))
    ).toBe(true)
  })
  it('keeps a free group free and omits the price ladder', () => {
    render(
      <GroupMultiplierCard
        status={{
          ...status,
          base_ratio: 0,
          effective_ratio: 0,
          next_request_ratio: 0,
        }}
      />
    )
    expect(
      screen.getByText('Current group multiplier').parentElement
    ).toHaveTextContent('Free of charge')
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
  it('preserves saved tiers and the expected version when switching to balance mode', async () => {
    const user = userEvent.setup()
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const close = vi.fn()
    render(
      <QueryClientProvider client={client}>
        <GroupPolicyDialog status={status} onClose={close} />
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('combobox', { name: 'Multiplier mode' }))
    await user.click(screen.getByRole('option', { name: 'Balance multiplier' }))
    expect(
      screen.queryByLabelText('Dynamic multiplier 1')
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/dynamic_ratio/policies', {
        group: 'Coding',
        policy: { ...status.policy, mode: 'balance' },
        expected_version: 'version-1',
      })
    )
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
  })
  it('rejects duplicate thresholds and preserves decimal multiplier input', async () => {
    const user = userEvent.setup()
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    render(
      <QueryClientProvider client={client}>
        <GroupPolicyDialog status={status} onClose={vi.fn()} />
      </QueryClientProvider>
    )
    fireEvent.change(screen.getByLabelText('Concurrency threshold 2'), {
      target: { value: '0' },
    })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText(
        'Thresholds must start at 0 and increase, up to 1000000.'
      )
    ).toBeVisible()
    expect(put).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Concurrency threshold 2'), {
      target: { value: '8' },
    })
    const multiplier = screen.getByLabelText(
      'Dynamic multiplier 1'
    ) as HTMLInputElement
    await user.clear(multiplier)
    await user.type(multiplier, '1.25')
    expect(multiplier.value).toBe('1.25')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/dynamic_ratio/policies',
        expect.objectContaining({
          policy: expect.objectContaining({
            tiers: [
              { minimum: 0, multiplier: 1.25 },
              ...status.policy.tiers.slice(1),
            ],
          }),
        })
      )
    )
  })
  it('keeps the editor open and retains the draft when saving fails', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'put').mockRejectedValue(new Error('configuration changed'))
    const close = vi.fn()
    render(
      <QueryClientProvider client={client}>
        <GroupPolicyDialog status={status} onClose={close} />
      </QueryClientProvider>
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    )
    expect(close).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Concurrency threshold 2')).toHaveValue(8)
  })
  it('shows user-visible groups without exposing configuration controls', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: { success: true, data: [status] } })
    render(
      <QueryClientProvider client={client}>
        <GroupMultiplierPanel />
      </QueryClientProvider>
    )
    expect(await screen.findByText('Coding')).toBeInTheDocument()
    expect(get).toHaveBeenCalledWith(
      '/api/dynamic_ratio/groups',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(
      screen.queryByRole('button', { name: 'Configure multiplier' })
    ).not.toBeInTheDocument()
  })
  it('updates multiplier formatting when the interface language changes', async () => {
    render(
      <GroupMultiplierCard status={{ ...status, effective_ratio: 1.25 }} />
    )
    expect(
      screen.getByText('Current group multiplier').parentElement
    ).toHaveTextContent('×1.25')
    await act(async () => {
      await i18next.changeLanguage('fr')
    })
    expect(
      screen.getByText('Current group multiplier').parentElement
    ).toHaveTextContent('×1,25')
  })
  it.each(['zhCN', 'zhTW', 'en', 'fr', 'ja', 'ru', 'vi', 'invalid_locale'])(
    'formats small multipliers for %s without rounding them to zero',
    (language) => {
      const result = formatMultiplier(0.0001, toIntlLocale(language))
      expect(result).toMatch(/1/)
      expect(result).not.toBe('×0')
    }
  )
  it('validates the tier ceiling and keeps fixed mode usable without tiers', () => {
    const schema = groupPolicySchema(i18next.t)
    expect(schema.safeParse({ mode: 'fixed', tiers: [] }).success).toBe(true)
    expect(schema.safeParse({ mode: 'concurrency', tiers: [] }).success).toBe(
      false
    )
    expect(
      schema.safeParse({
        mode: 'concurrency',
        tiers: [{ minimum: 0, multiplier: 1001 }],
      }).success
    ).toBe(false)
  })
})
