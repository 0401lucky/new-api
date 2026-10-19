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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { WalletStatsCard } from '../components/wallet-stats-card'
import { BillingHistoryDialog } from '../components/dialogs/billing-history-dialog'
import type { UserWalletData } from '../types'

const user: UserWalletData = {
  id: 1,
  username: 'journal-user',
  quota: 2500000,
  temporary_quota: 50000,
  temporary_quota_expires_at_display: '10-18 23:59',
  used_quota: 1200000,
  request_count: 42,
  aff_quota: 0,
  aff_history_quota: 0,
  aff_count: 0,
  group: 'default',
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'journal-user',
    role: 1,
  })
})

afterEach(() => {
  cleanup()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  vi.restoreAllMocks()
})

describe('journal wallet', () => {
  it('keeps every passbook figure visible and hides the paper decorations', () => {
    render(<WalletStatsCard user={user} timezone='Asia/Shanghai' />)

    for (const label of [
      'Permanent balance',
      "Today's limited-time quota",
      'Total Usage',
      'API Requests',
    ]) {
      expect(screen.getByText(label)).toBeVisible()
    }
    expect(screen.getByText('42')).toBeVisible()
    expect(screen.getByText(/Expires at 10-18 23:59/)).toBeVisible()

    const stats = document.querySelector('[data-slot="wallet-stats"]')
    expect(stats).not.toBeNull()
    expect(stats?.querySelectorAll('[data-slot="wallet-stat"]')).toHaveLength(4)

    const decorations = stats?.querySelectorAll(
      '.journal-tape, .journal-paper-clip'
    )
    expect(decorations).toHaveLength(2)
    decorations?.forEach((node) => {
      expect(node).toHaveAttribute('aria-hidden', 'true')
      expect(node).toHaveClass('pointer-events-none')
    })
  })

  it('renders billing records as paper receipts with stamp statuses and unchanged totals', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          total: 2,
          items: [
            {
              id: 1,
              user_id: 1,
              trade_no: 'JOURNAL-001',
              status: 'success',
              amount: 20,
              money: 146,
              payment_method: 'alipay',
              create_time: 1,
            },
            {
              id: 2,
              user_id: 1,
              trade_no: 'JOURNAL-002',
              status: 'pending',
              amount: 10,
              money: 73,
              payment_method: 'wxpay',
              create_time: 2,
            },
          ],
        },
      },
    })

    render(<BillingHistoryDialog open onOpenChange={vi.fn()} />)

    expect(await screen.findByText('JOURNAL-001')).toBeVisible()
    expect(screen.getByText(/Showing\s+1-2\s+of\s+2/)).toBeVisible()

    const list = document.querySelector('[data-slot="billing-history-list"]')
    expect(list).not.toBeNull()
    expect(
      list?.querySelectorAll('[data-slot="billing-record"]')
    ).toHaveLength(2)
    const badges = list?.querySelectorAll('[data-slot="status-badge"]')
    expect(badges).toHaveLength(2)
    badges?.forEach((badge) => {
      expect(badge).toHaveAttribute('data-type', 'badge')
    })
  })
})
