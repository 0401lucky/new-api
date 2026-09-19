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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema } from '../../types'
import { ChannelRowActionsLayoutContext } from '../channel-row-actions-context'
import { BalanceCell } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'

const originalAuth = useAuthStore.getState().auth
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState({ auth: originalAuth })
})

it('shows per-model call counts when opening the used quota cell', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/data/channel_model_usage') {
      return {
        data: {
          success: true,
          data: {
            channel_id: 42,
            start_timestamp: 0,
            end_timestamp: 0,
            models: [
              { model_name: 'gpt-a', request_count: 3 },
              { model_name: 'gpt-b', request_count: 1 },
            ],
            total_requests: 4,
          },
        },
      }
    }
    return { data: { success: true, data: [] } }
  })
  const channel = channelSchema.parse({
    id: 42,
    type: 1,
    key: '',
    name: 'Test Channel',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    used_quota: 1234,
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelRowActionsLayoutContext.Provider value='table'>
          <BalanceCell channel={channel} />
        </ChannelRowActionsLayoutContext.Provider>
      </ChannelsProvider>
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Channel Model Usage' }))

  expect(
    await screen.findByRole('dialog', { name: 'Channel Model Usage' })
  ).toBeInTheDocument()
  expect(await screen.findByText('gpt-a')).toBeInTheDocument()
  expect(await screen.findByText('gpt-b')).toBeInTheDocument()
  expect(get).toHaveBeenCalledWith(
    '/api/data/channel_model_usage',
    expect.objectContaining({ params: { channel_id: 42 } })
  )
  // 已用额度只用于查看统计，不应触发余额更新。
  expect(get.mock.calls.some(([url]) => String(url).includes('update_balance'))).toBe(
    false
  )
})

it('opens the sheet with the keyboard from the used quota cell', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { channel_id: 42, models: [], total_requests: 0 } },
  })
  const channel = channelSchema.parse({
    id: 42,
    type: 1,
    key: '',
    name: 'Test Channel',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelRowActionsLayoutContext.Provider value='card'>
          <BalanceCell channel={channel} />
        </ChannelRowActionsLayoutContext.Provider>
      </ChannelsProvider>
    </QueryClientProvider>
  )

  const entry = screen.getByRole('button', { name: 'Channel Model Usage' })
  expect(entry).toHaveAttribute('aria-haspopup', 'dialog')
  entry.focus()
  await user.keyboard('{Enter}')

  expect(
    await screen.findByRole('dialog', { name: 'Channel Model Usage' })
  ).toBeInTheDocument()
  expect(await screen.findByText('No data')).toBeInTheDocument()
})
