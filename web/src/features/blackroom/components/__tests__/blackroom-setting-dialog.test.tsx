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

import type { BlackroomSetting, BlackroomStatusSummary } from '../../types'
import { BlackroomProvider, useBlackroom } from '../blackroom-provider'
import { BlackroomSettingDialog } from '../blackroom-setting-dialog'

const clients: QueryClient[] = []

const SETTING: BlackroomSetting = {
  enabled: true,
  auto_ban_enabled: true,
  lookback_hours: 12,
  check_interval_minutes: 5,
  min_requests: 2,
  rules: [{ ip_count: 10, duration_hours: 24, permanent: false }],
  escalation_window_days: 14,
  escalation_temporary_ban_count: 2,
  exempt_user_ids: [7],
  exempt_groups: ['vip'],
  shadow_mode: true,
  realtime_enabled: false,
  geo_enabled: true,
  geo_country_count: 4,
  geo_asn_count: 5,
  geo_min_gap_seconds: 90,
  geo_duration_hours: 48,
  country_mmdb_path: '/data/GeoLite2-Country.mmdb',
  asn_mmdb_path: '/data/GeoLite2-ASN.mmdb',
}

function buildStatus(
  overrides: Partial<BlackroomStatusSummary> = {}
): BlackroomStatusSummary {
  return {
    enabled: true,
    auto_ban_enabled: true,
    shadow_mode: true,
    realtime_enabled: false,
    geo_enabled: true,
    geo_effective: false,
    resolver: {
      ready: false,
      country_ready: false,
      asn_ready: false,
      version: '',
      error_code: 'not_configured',
    },
    blocking: ['geo_resolver_not_ready'],
    ...overrides,
  }
}

function OpenSettingsButton() {
  const { setOpen } = useBlackroom()

  return (
    <Button type='button' onClick={() => setOpen('setting')}>
      open settings
    </Button>
  )
}

function renderSettingsDialog(status: BlackroomStatusSummary) {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/blackroom/status') {
      return { data: { success: true, data: status } }
    }
    return { data: { success: true, data: SETTING } }
  })
  vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, data: SETTING },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)

  return render(
    <QueryClientProvider client={client}>
      <BlackroomProvider>
        <BlackroomSettingDialog />
        <OpenSettingsButton />
      </BlackroomProvider>
    </QueryClientProvider>
  )
}

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'open settings' }))
  await screen.findByDisplayValue('/data/GeoLite2-Country.mmdb')
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})

test('loads the stored shadow and geo configuration into the form', async () => {
  const user = userEvent.setup()
  renderSettingsDialog(buildStatus())

  await openSettings(user)

  expect(screen.getByRole('switch', { name: 'Shadow mode' })).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Enable geo blocking' })
  ).toBeChecked()
  expect(screen.getByLabelText('Country count')).toHaveValue(4)
  expect(screen.getByLabelText('Geo ban duration hours')).toHaveValue(48)
  expect(screen.getByLabelText('ASN MMDB path')).toHaveValue(
    '/data/GeoLite2-ASN.mmdb'
  )
})

test('explains why geo blocking is not effective when the MMDB path is missing', async () => {
  const user = userEvent.setup()
  renderSettingsDialog(buildStatus())

  await openSettings(user)

  expect(screen.getByText('Not effective')).toBeInTheDocument()
  expect(screen.getByText('No MMDB path is configured.')).toBeInTheDocument()
})

test('reports an effective geo resolver without a blocking reason', async () => {
  const user = userEvent.setup()
  renderSettingsDialog(
    buildStatus({
      geo_effective: true,
      resolver: {
        ready: true,
        country_ready: true,
        asn_ready: true,
        version: 'country-1/asn-2',
      },
      blocking: [],
    })
  )

  await openSettings(user)

  expect(screen.getByText('Effective')).toBeInTheDocument()
  expect(
    screen.queryByText('No MMDB path is configured.')
  ).not.toBeInTheDocument()
})

test('submits the edited shadow, realtime, and geo fields with the setting', async () => {
  const user = userEvent.setup()
  renderSettingsDialog(buildStatus())

  await openSettings(user)
  await user.click(screen.getByRole('switch', { name: 'Shadow mode' }))
  await user.click(screen.getByRole('switch', { name: 'Realtime blocking' }))
  const countryCount = screen.getByLabelText('Country count')
  await user.clear(countryCount)
  await user.type(countryCount, '7')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  expect(api.put).toHaveBeenCalledTimes(1)
  const [url, payload] = vi.mocked(api.put).mock.calls[0]
  expect(url).toBe('/api/blackroom/setting')
  expect(payload).toMatchObject({
    shadow_mode: false,
    realtime_enabled: true,
    geo_enabled: true,
    geo_country_count: 7,
    geo_asn_count: 5,
    geo_min_gap_seconds: 90,
    geo_duration_hours: 48,
    country_mmdb_path: '/data/GeoLite2-Country.mmdb',
    asn_mmdb_path: '/data/GeoLite2-ASN.mmdb',
  })
})
