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
import {
  QueryClient,
  QueryClientProvider,
  focusManager,
} from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { RateLimitUsage } from '..'
import type { RateLimitUsageData } from '../api'

const initial: RateLimitUsageData = {
  enabled: true,
  exempt: false,
  window_minutes: 1,
  total_mode: 'sliding_window',
  groups: [
    {
      group: 'Free',
      total: { used: null, limit: 0 },
      success: { used: 8, limit: 30 },
      concurrency: { used: 2, limit: 4 },
    },
    {
      group: 'CC',
      total: { used: 100, limit: 90 },
      success: { used: 8, limit: 60 },
      concurrency: { used: null, limit: 0 },
    },
  ],
}

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 42, username: 'alice', role: 1 })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
  focusManager.setFocused(undefined)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

function renderPage() {
  return render(
    <QueryClientProvider client={client}>
      <RateLimitUsage />
    </QueryClientProvider>
  )
}

describe('rate limit usage', () => {
  it('shows each group limit, caps saturated progress, and distinguishes unlimited metrics', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: initial },
    })
    renderPage()
    expect(await screen.findByText('Free')).toBeVisible()
    expect(screen.getByText('8 / 30')).toBeVisible()
    expect(screen.getByText('2 / 4')).toBeVisible()
    expect(screen.getByText('100 / 90')).toBeVisible()
    expect(screen.getByText('Unlimited')).toBeVisible()
    expect(
      screen.getByRole('progressbar', { name: 'Total requests / 1 min' })
    ).toHaveAttribute('aria-valuenow', '100')
    expect(screen.getAllByRole('progressbar')).toHaveLength(5)
    expect(screen.queryByText(/tokens per minute/i)).not.toBeInTheDocument()
    expect(screen.getByText('Free').closest('[data-slot="card"]')).toHaveClass(
      'min-w-0'
    )
    expect(
      screen.getByText('CC').closest('[data-slot="card"]')?.parentElement
    ).toHaveClass('md:grid-cols-2')
  })

  it('shows loading, then an error on failed refresh, and recovers after retry', async () => {
    let resolveRequest!: (value: unknown) => void
    vi.spyOn(api, 'get')
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveRequest = resolve
          })
      )
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ data: { success: true, data: initial } })
    renderPage()
    expect(screen.getByText('Loading...')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeDisabled()
    await act(async () => {
      resolveRequest({ data: { success: true, data: initial } })
    })
    expect(await screen.findByText('8 / 30')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(
      await screen.findByText('Failed to load rate limit usage')
    ).toBeVisible()
    expect(screen.queryByText('8 / 30')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('8 / 30')).toBeVisible()
  })

  it.each([
    [{ ...initial, enabled: false }, 'User rate limits are disabled'],
    [
      { ...initial, exempt: true },
      'Your account is exempt from user rate limits',
    ],
    [{ ...initial, groups: [] }, 'No available groups'],
  ])(
    'shows an explicit state instead of zero counters for %j',
    async (data, title) => {
      vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data } })
      renderPage()
      expect(await screen.findByText(title)).toBeVisible()
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    }
  )

  it('treats success false as an error instead of displaying empty usage', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: false, message: 'unavailable' },
    })
    renderPage()
    expect(
      await screen.findByText('Failed to load rate limit usage')
    ).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })

  it('uses the configured window and distinguishes bucket occupancy from rolling requests', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { ...initial, window_minutes: 5, total_mode: 'token_bucket' },
      },
    })
    renderPage()
    expect(await screen.findByText('Window: 5 min')).toBeVisible()
    expect(
      screen.getAllByRole('progressbar', {
        name: 'Successful requests / 5 min',
      })
    ).toHaveLength(2)
    expect(
      screen.getByRole('progressbar', { name: 'Total request limit usage' })
    ).toBeVisible()
  })

  it('refreshes after five seconds and suspends polling when the page is hidden', async () => {
    vi.useFakeTimers()
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data: initial } })
      .mockResolvedValue({
        data: {
          success: true,
          data: {
            ...initial,
            groups: [
              { ...initial.groups[0], concurrency: { used: 3, limit: 4 } },
            ],
          },
        },
      })
    await act(async () => {
      renderPage()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(screen.getByText('2 / 4')).toBeVisible()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(screen.getByText('3 / 4')).toBeVisible()
    focusManager.setFocused(false)
    get.mockClear()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(get).not.toHaveBeenCalled()
  })

  it('wraps a long group name without dropping its usage', async () => {
    const group = 'long-group-name-'.repeat(12)
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { ...initial, groups: [{ ...initial.groups[0], group }] },
      },
    })
    renderPage()
    const name = await screen.findByText(group)
    expect(name).toHaveClass('break-all')
    expect(
      within(name.closest('[data-slot="card"]') as HTMLElement).getByText(
        '8 / 30'
      )
    ).toBeVisible()
  })
})
