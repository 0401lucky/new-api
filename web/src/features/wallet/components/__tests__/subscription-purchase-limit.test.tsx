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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { UserSubscriptionRecord } from '@/features/subscriptions/types'
import { api } from '@/lib/api'

import { SubscriptionPlansCard } from '../subscription-plans-card'

const now = 2_000_000_000

function subscription(
  overrides: Partial<UserSubscriptionRecord['subscription']> = {}
): UserSubscriptionRecord {
  return {
    subscription: {
      id: 1,
      user_id: 1,
      plan_id: 1,
      status: 'active',
      start_time: now - 3600,
      end_time: now + 3600,
      amount_total: 1000,
      amount_used: 0,
      ...overrides,
    },
  }
}

function renderPlans(
  allSubscriptions: UserSubscriptionRecord[],
  activeSubscriptions: UserSubscriptionRecord[] = [],
  limit = 1
) {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: [
          {
            plan: {
              id: 1,
              title: 'Renewable plan',
              price_amount: 2,
              currency: 'USD',
              duration_unit: 'day',
              duration_value: 7,
              quota_reset_period: 'never',
              enabled: true,
              sort_order: 0,
              max_purchase_per_user: limit,
              total_amount: 1000,
            },
          },
        ],
      },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          billing_preference: 'wallet_first',
          subscriptions: activeSubscriptions,
          all_subscriptions: allSubscriptions,
        },
      },
    })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SubscriptionPlansCard topupInfo={null} userQuota={5_000_000} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
})

describe('subscription purchase limit', () => {
  it.each([
    ['expired before status cleanup', { end_time: now - 1 }],
    ['at the expiry boundary', { end_time: now }],
    ['marked expired', { status: 'expired' }],
    ['cancelled', { status: 'cancelled' }],
  ])(
    'allows renewal when the previous subscription is %s',
    async (_, overrides) => {
      renderPlans([subscription(overrides)])
      const user = userEvent.setup()
      await user.click(
        await screen.findByRole('button', { name: 'Subscribe Now' })
      )
      const dialog = await screen.findByRole('dialog', {
        name: 'Purchase Subscription',
      })
      expect(
        within(dialog).getByRole('button', { name: 'Pay with Balance' })
      ).toBeEnabled()
    }
  )

  it.each([0, 1000])(
    'blocks another purchase while the subscription is active with %i quota used',
    async (used) => {
      const active = subscription({ amount_used: used })
      renderPlans([active], [active])
      expect(
        await screen.findByRole('button', { name: 'Limit Reached' })
      ).toBeDisabled()
      expect(
        screen.queryByRole('button', { name: 'Subscribe Now' })
      ).not.toBeInTheDocument()
    }
  )

  it('allows a remaining active slot despite expired history and another plan', async () => {
    const active = subscription()
    const otherPlan = subscription({ id: 2, plan_id: 2 })
    const expired = subscription({ id: 3, end_time: now - 1 })
    renderPlans([active, otherPlan, expired], [active, otherPlan], 2)
    expect(
      await screen.findByRole('button', { name: 'Subscribe Now' })
    ).toBeEnabled()
  })

  it('blocks purchases when two active subscriptions fill a limit of two', async () => {
    const active = [subscription(), subscription({ id: 2 })]
    renderPlans(active, active, 2)
    expect(
      await screen.findByRole('button', { name: 'Limit Reached' })
    ).toBeDisabled()
  })

  it('allows another purchase when the limit is zero', async () => {
    const active = subscription()
    renderPlans([active], [active], 0)
    expect(
      await screen.findByRole('button', { name: 'Subscribe Now' })
    ).toBeEnabled()
  })
})
