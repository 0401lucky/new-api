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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { LogStatCards } from '../log-stat-cards'
import { ModelUsageTable } from '../model-usage-table'
import { PerformanceOverview } from '../performance-overview'

let client: QueryClient

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'dashboard-user',
    role: 1,
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/data/self') {
      return {
        data: {
          success: true,
          data: [
            {
              model_name: 'alpha',
              created_at: Math.floor(Date.now() / 1000),
              count: 5,
              quota: 100,
              token_used: 1000,
            },
          ],
        },
      }
    }
    if (url === '/api/perf-metrics/summary') {
      return {
        data: {
          success: true,
          data: {
            summary: {
              success_rate: 95.87,
              avg_latency_ms: 24220,
              avg_tps: 103.8,
            },
            models: [
              {
                model_name: 'alpha',
                success_rate: 96.97,
                avg_latency_ms: 22000,
                avg_tps: 110.2,
              },
            ],
          },
        },
      }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  vi.restoreAllMocks()
})

describe('journal model analytics', () => {
  it('keeps all five KPI values readable without decorations in the data strip', async () => {
    render(
      <QueryClientProvider client={client}>
        <LogStatCards />
      </QueryClientProvider>
    )

    expect(await screen.findByText('Total Count')).toBeVisible()
    expect(await screen.findByText('5')).toBeVisible()
    const strip = document.querySelector('[data-slot="dashboard-stat-strip"]')
    expect(strip).not.toBeNull()
    expect(
      strip?.querySelectorAll('[data-slot="dashboard-stat-cell"]')
    ).toHaveLength(5)

    const decorations = strip?.querySelectorAll(
      '.journal-tape, .journal-paper-clip'
    )
    expect(decorations).toHaveLength(0)
  })

  it('keeps performance metrics and model success rates readable inside the paper card', async () => {
    render(
      <QueryClientProvider client={client}>
        <PerformanceOverview />
      </QueryClientProvider>
    )

    expect(await screen.findByText('96.97%')).toBeVisible()
    expect(screen.getByText('95.87%')).toBeVisible()
    expect(screen.getByText('24.22s')).toBeVisible()
    expect(screen.getByText('103.8 t/s')).toBeVisible()

    const panel = document.querySelector('[data-slot="dashboard-performance"]')
    expect(panel).not.toBeNull()
    expect(
      panel?.querySelector('[data-slot="dashboard-performance-model"]')
    ).toHaveTextContent('96.97%')
    expect(panel?.querySelector('.journal-tape')).toBeNull()
  })

  it('gives the model usage detail table its paper anchor without changing its data', () => {
    render(
      <ModelUsageTable
        data={[
          { model_name: 'alpha', created_at: 1, count: 3, token_used: 120 },
        ]}
      />
    )

    expect(
      screen.getByRole('region', { name: 'Model Usage Details' })
    ).toHaveAttribute('data-slot', 'dashboard-usage-table')
    expect(screen.getByRole('cell', { name: 'alpha' })).toBeVisible()
  })
})
