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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { Subscriptions } from '../../index'

let client: QueryClient

const now = Math.floor(Date.now() / 1000)

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const target = String(url)
    if (target.includes('/api/subscription/admin/plans')) {
      return {
        data: {
          success: true,
          data: [
            {
              plan: {
                id: 1,
                title: '四叶草月卡',
                subtitle: '适合日常开发',
                price_amount: 29.9,
                total_amount: 3000000,
                duration_unit: 'month',
                duration_value: 1,
                reset_period: 'monthly',
                enabled: true,
                sort_order: 1,
              },
            },
          ],
        },
      }
    }
    if (target.includes('/api/subscription/admin/user_subscriptions')) {
      return {
        data: {
          success: true,
          data: {
            page: 1,
            page_size: 20,
            total: 1,
            items: [
              {
                subscription: {
                  id: 7,
                  user_id: 2,
                  plan_id: 1,
                  amount_total: 3000000,
                  amount_used: 500000,
                  status: 'active',
                  start_time: now - 1000,
                  end_time: now + 864000,
                  next_reset_time: 0,
                  source: 'order',
                },
                username: 'clover-user',
                plan_title: '四叶草月卡',
              },
            ],
          },
        },
      }
    }
    if (target.includes('/api/option')) {
      return { data: { success: true, data: [] } }
    }
    return { data: { success: true, data: {} } }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

describe('journal subscriptions', () => {
  it('marks the page as immersive and keeps plan and instance tables on paper', async () => {
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <Subscriptions />
      </QueryClientProvider>
    )

    expect(await screen.findByText('四叶草月卡')).toBeVisible()
    expect(screen.getByRole('main')).toHaveAttribute(
      'data-journal-level',
      'immersive'
    )

    await user.click(
      screen.getByRole('tab', { name: 'User subscriptions' })
    )
    expect(await screen.findByText('clover-user')).toBeVisible()

    const table = document.querySelector(
      '[data-slot="user-subscriptions-table"]'
    )
    expect(table).not.toBeNull()
    expect(table).toHaveTextContent('Active')
    expect(table).toHaveTextContent('17%')
    expect(
      table?.querySelector('[data-slot="status-badge"]')
    ).toHaveAttribute('data-type', 'badge')
  })
})
