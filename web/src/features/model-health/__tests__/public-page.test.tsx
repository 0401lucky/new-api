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
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { getPublicModelHealthOverview } from '../api'
import { ModelHealthPublicPage } from '../public-page'
import type {
  ApiEnvelope,
  ModelHealthOverviewPayload,
  ModelHealthPeriod,
} from '../types'

vi.mock('../api', () => ({ getPublicModelHealthOverview: vi.fn() }))
vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => (
    <main>{props.children}</main>
  ),
}))

afterEach(() => vi.resetAllMocks())

function response(
  period: ModelHealthPeriod
): ApiEnvelope<ModelHealthOverviewPayload> {
  return {
    success: true,
    data: {
      updated_at: 7200,
      observed_since: 3600,
      period,
      global_status: 'operational',
      stats: {
        total_models: 1,
        healthy_models: 1,
        overall_rate_24h: 1,
        total_tokens_24h: 100,
      },
      models: [
        {
          model_name: `model-${period}`,
          status: 'operational',
          availability: 1,
          availability_success: 22,
          availability_total: 22,
          avg_latency_ms: null,
          avg_ttft_ms: null,
          success_tokens_24h: 100,
          timeline: [],
        },
      ],
    },
  }
}

test('switching the period prevents a slower previous response from replacing the selected period', async () => {
  let resolvePrevious!: (value: ApiEnvelope<ModelHealthOverviewPayload>) => void
  const previous = new Promise<ApiEnvelope<ModelHealthOverviewPayload>>(
    (resolve) => {
      resolvePrevious = resolve
    }
  )
  vi.mocked(getPublicModelHealthOverview).mockImplementation((period) =>
    period === '7d' ? previous : Promise.resolve(response(period))
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  const view = render(
    <QueryClientProvider client={client}>
      <ModelHealthPublicPage />
    </QueryClientProvider>
  )
  await user.click(screen.getByRole('button', { name: '15 days' }))
  expect(await screen.findByText('model-15d')).toBeVisible()
  await act(async () => resolvePrevious(response('7d')))
  expect(screen.getByText('model-15d')).toBeVisible()
  expect(screen.queryByText('model-7d')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '15 days' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  view.unmount()
  client.clear()
})

test('a new installation shows no data and does not claim that every model is operational', async () => {
  const empty = response('7d')
  empty.data.models = []
  empty.data.global_status = 'no_data'
  empty.data.observed_since = null
  empty.data.stats = {
    total_models: 0,
    healthy_models: 0,
    overall_rate_24h: 0,
    total_tokens_24h: 0,
  }
  vi.mocked(getPublicModelHealthOverview).mockResolvedValue(empty)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <ModelHealthPublicPage />
    </QueryClientProvider>
  )
  expect((await screen.findAllByText('No data')).length).toBeGreaterThan(0)
  expect(screen.queryByText('All systems operational')).not.toBeInTheDocument()
  expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
  view.unmount()
  client.clear()
})
