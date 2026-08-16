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
import { render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { formatQuota } from '@/lib/format'

import { getUserInfo } from '../../api'
import { UserInfoDialog } from '../dialogs/user-info-dialog'

vi.mock('../../api', () => ({
  getUserInfo: vi.fn(),
}))

const mockedGetUserInfo = vi.mocked(getUserInfo)

function getLimitedTimeQuotaItem() {
  const label = screen.getByText("Today's limited-time quota")
  const item = label.closest('div')
  if (!item) {
    throw new Error('Limited-time quota item not found')
  }
  return item
}

describe('UserInfoDialog limited-time quota', () => {
  beforeEach(() => {
    mockedGetUserInfo.mockReset()
  })

  test('shows active limited-time quota and expiry when present', async () => {
    mockedGetUserInfo.mockResolvedValue({
      success: true,
      data: {
        id: 1,
        username: 'user1',
        quota: 1000,
        used_quota: 500,
        request_count: 10,
        temporary_quota: 3000,
        temporary_quota_expires_at_display: '01-02 15:04',
      },
    })

    render(<UserInfoDialog userId={1} open onOpenChange={() => {}} />)

    expect(
      await screen.findByText("Today's limited-time quota")
    ).toBeInTheDocument()
    expect(
      within(getLimitedTimeQuotaItem()).getByText(formatQuota(3000))
    ).toBeInTheDocument()
    expect(screen.getByText('Expires at')).toBeInTheDocument()
    expect(screen.getByText('01-02 15:04')).toBeInTheDocument()
  })

  test('shows zero limited-time quota when not present', async () => {
    mockedGetUserInfo.mockResolvedValue({
      success: true,
      data: {
        id: 2,
        username: 'user2',
        quota: 0,
        used_quota: 0,
        request_count: 0,
      },
    })

    render(<UserInfoDialog userId={2} open onOpenChange={() => {}} />)

    expect(
      await screen.findByText("Today's limited-time quota")
    ).toBeInTheDocument()
    expect(
      within(getLimitedTimeQuotaItem()).getByText(formatQuota(0))
    ).toBeInTheDocument()
    expect(screen.queryByText('Expires at')).not.toBeInTheDocument()
  })
})
