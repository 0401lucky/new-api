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
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { BlackroomStatusSummary } from '../../types'
import { BlackroomStatusBanner } from '../blackroom-status-banner'

const clients: QueryClient[] = []

function buildStatus(
  overrides: Partial<BlackroomStatusSummary> = {}
): BlackroomStatusSummary {
  return {
    enabled: true,
    auto_ban_enabled: true,
    shadow_mode: false,
    realtime_enabled: true,
    geo_enabled: true,
    geo_effective: true,
    resolver: {
      ready: true,
      country_ready: true,
      asn_ready: true,
      version: 'country-1/asn-2',
    },
    blocking: [],
    ...overrides,
  }
}

async function renderBanner(status: BlackroomStatusSummary) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: status },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)

  return render(
    <QueryClientProvider client={client}>
      <BlackroomStatusBanner />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})

test('shows geo blocking as effective and hides the blocker alert when nothing blocks auto bans', async () => {
  await renderBanner(buildStatus())

  expect(await screen.findByText('Effective')).toBeInTheDocument()
  expect(
    screen.queryByText('Automatic bans are currently blocked')
  ).not.toBeInTheDocument()
})

test('explains an unready MMDB resolver instead of claiming geo blocking works', async () => {
  await renderBanner(
    buildStatus({
      geo_effective: false,
      resolver: {
        ready: false,
        country_ready: false,
        asn_ready: false,
        version: '',
        error_code: 'not_configured',
      },
      blocking: ['geo_resolver_not_ready'],
    })
  )

  expect(await screen.findByText('Not effective')).toBeInTheDocument()
  expect(
    screen.getByText('Automatic bans are currently blocked')
  ).toBeInTheDocument()
  expect(
    screen.getByText(
      'The MMDB resolver is not ready, so geo blocking is skipped.'
    )
  ).toBeInTheDocument()
})

test('flags shadow mode and disabled auto bans as the reason no ban is applied', async () => {
  await renderBanner(
    buildStatus({
      auto_ban_enabled: false,
      shadow_mode: true,
      geo_enabled: false,
      geo_effective: false,
      blocking: ['auto_ban_disabled'],
    })
  )

  expect(await screen.findByText('Recording only')).toBeInTheDocument()
  expect(screen.getByText('Not effective')).toBeInTheDocument()
  expect(
    screen.getByText('Auto ban is disabled, so only manual bans apply.')
  ).toBeInTheDocument()
})
