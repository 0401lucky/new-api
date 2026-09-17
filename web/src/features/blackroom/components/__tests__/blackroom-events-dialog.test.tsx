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
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'

import type { BlackroomBanEvent } from '../../types'
import { BlackroomEventsDialog } from '../blackroom-events-dialog'
import { BlackroomProvider, useBlackroom } from '../blackroom-provider'

const clients: QueryClient[] = []

const EVENTS: BlackroomBanEvent[] = [
  {
    id: 3,
    ban_id: 11,
    user_id: 42,
    event_type: 'extend',
    source: 'auto',
    reason: 'Extended after repeated matches',
    evidence: '{"distinct_ips":9,"country_count":3}',
    ip_count: 9,
    ip_list: '["1.1.1.1","2.2.2.2"]',
    window_start: 1_754_800_000,
    window_end: 1_754_900_000,
    ban_duration_seconds: 259_200,
    banned_until: 1_755_200_000,
    actor_user_id: 0,
    created_at: 1_754_900_000,
  },
  {
    id: 2,
    ban_id: 11,
    user_id: 42,
    event_type: 'apply',
    source: 'manual',
    reason: '',
    evidence: '',
    ip_count: 8,
    ip_list: '',
    window_start: 0,
    window_end: 0,
    ban_duration_seconds: 0,
    banned_until: 0,
    actor_user_id: 7,
    created_at: 1_754_800_000,
  },
  {
    id: 1,
    ban_id: 0,
    user_id: 42,
    event_type: 'shadow_match',
    source: 'auto',
    reason: 'Shadow hit',
    evidence: '',
    ip_count: 8,
    ip_list: '',
    window_start: 0,
    window_end: 0,
    ban_duration_seconds: 0,
    banned_until: 0,
    actor_user_id: 0,
    created_at: 1_754_700_000,
  },
]

function OpenEventsButton() {
  const { setCurrentRow, setOpen } = useBlackroom()

  return (
    <Button
      type='button'
      onClick={() => {
        setCurrentRow({ id: 11, user_id: 42, username: 'alice' })
        setOpen('events')
      }}
    >
      open events
    </Button>
  )
}

function renderDialog(events: BlackroomBanEvent[]) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: events },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)

  return render(
    <QueryClientProvider client={client}>
      <BlackroomProvider>
        <BlackroomEventsDialog />
        <OpenEventsButton />
      </BlackroomProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})

test('lists every ban event for the user in a timeline', async () => {
  const user = userEvent.setup()
  renderDialog(EVENTS)

  await user.click(screen.getByRole('button', { name: 'open events' }))

  expect(await screen.findByText('Ban extended')).toBeInTheDocument()
  expect(screen.getByText('Ban applied')).toBeInTheDocument()
  expect(screen.getByText('Shadow match')).toBeInTheDocument()
  expect(
    screen.getByText('Extended after repeated matches')
  ).toBeInTheDocument()
  expect(screen.getByText('No reason recorded')).toBeInTheDocument()
})

test('shows permanence, the extend duration, and the acting operator per event', async () => {
  const user = userEvent.setup()
  renderDialog(EVENTS)

  await user.click(screen.getByRole('button', { name: 'open events' }))

  expect(await screen.findByText(/72 hours/)).toBeInTheDocument()
  expect(screen.getByText(/Ban duration: Permanent/)).toBeInTheDocument()
  expect(screen.getByText(/Operator: User 7/)).toBeInTheDocument()
  expect(screen.getAllByText(/Operator: System/)).toHaveLength(2)
})

test('keeps evidence collapsed until it is expanded', async () => {
  const user = userEvent.setup()
  renderDialog(EVENTS)

  await user.click(screen.getByRole('button', { name: 'open events' }))

  const trigger = await screen.findByRole('button', { name: 'Evidence' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByText(/"distinct_ips": 9/)).not.toBeInTheDocument()

  await user.click(trigger)

  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText(/"distinct_ips": 9/)).toBeInTheDocument()
})

test('shows the empty state when the user has no recorded event', async () => {
  const user = userEvent.setup()
  renderDialog([])

  await user.click(screen.getByRole('button', { name: 'open events' }))

  expect(await screen.findByText('No ban events recorded')).toBeInTheDocument()
})
