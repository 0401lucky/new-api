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
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { PageFooterProvider } from '@/components/layout/components/page-footer'
import fr from '@/i18n/locales/fr.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { blackroomSearchSchema } from '../../lib/blackroom-search'
import type {
  ApiResponse,
  BlackroomDistributionItem,
  BlackroomDistributionResult,
} from '../../types'
import { BlackroomDistributionTable } from '../blackroom-distribution-table'
import { BlackroomProvider } from '../blackroom-provider'

const clients: QueryClient[] = []
const footers: HTMLElement[] = []
const requests: string[] = []
let selectUser: (userId: number) => void = () => undefined

const OBSERVATION: BlackroomDistributionItem = {
  user_id: 42,
  username: 'alice',
  score: 45,
  request_count: 1234,
  last_seen_at: 1_754_900_000,
  reasons: ['first_prompt_diversity', 'system_prompt_diversity'],
  tokens: [
    {
      token_id: 10,
      token_name: 'personal key',
      request_count: 1234,
      default_go_count: 0,
      hosting_go_count: 0,
      geo_sample_count: 500,
      system_prompt: { sample_count: 8, unique_count: 5, dominant_share: 0.25 },
      first_user_prompt: {
        sample_count: 8,
        unique_count: 6,
        dominant_share: 0.125,
      },
      subnets: ['8.8.8.0/24'],
      countries: ['US'],
      asns: [15169],
      prompt_limited: false,
      network_limited: false,
      score: 45,
      reasons: ['first_prompt_diversity', 'system_prompt_diversity'],
      first_seen_at: 1_754_899_400,
      last_seen_at: 1_754_900_000,
    },
  ],
}

const RESULT: BlackroomDistributionResult = {
  available: true,
  window_seconds: 600,
  prompt_window_seconds: 60,
  observed_at: 1_754_900_000,
  scope_limited: false,
  total: 1,
  page: 1,
  page_size: 20,
  items: [OBSERVATION],
}

function WatchlistHarness() {
  const search = watchlistRoute.useSearch()
  const navigate = watchlistRoute.useNavigate()
  return (
    <BlackroomProvider>
      <BlackroomDistributionTable
        search={search}
        navigate={navigate}
        onSelectUser={selectUser}
      />
    </BlackroomProvider>
  )
}

const rootRoute = createRootRoute()
const watchlistRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  validateSearch: blackroomSearchSchema,
  component: WatchlistHarness,
})

async function renderWatchlist(
  options: {
    result?: BlackroomDistributionResult
    initialEntry?: string
    response?: () => Promise<{ data: ApiResponse<BlackroomDistributionResult> }>
  } = {}
) {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    requests.push(url)
    if (options.response) return options.response()
    return { data: { success: true, data: options.result ?? RESULT } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const footer = document.createElement('div')
  document.body.append(footer)
  footers.push(footer)
  const router = createRouter({
    routeTree: rootRoute.addChildren([watchlistRoute]),
    history: createMemoryHistory({
      initialEntries: [options.initialEntry ?? '/'],
    }),
    defaultPendingMinMs: 0,
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <PageFooterProvider container={footer}>
        <RouterProvider router={router} />
      </PageFooterProvider>
    </QueryClientProvider>
  )
  return router
}

beforeEach(() => {
  localStorage.clear()
  requests.length = 0
  selectUser = () => undefined
})

afterEach(async () => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  footers.splice(0).forEach((footer) => footer.remove())
  await i18next.changeLanguage('en')
  for (const language of ['zhCN', 'zhTW', 'fr']) {
    i18next.removeResourceBundle(language, 'translation')
  }
})

test('shows account priority and supporting signals as review hints', async () => {
  await renderWatchlist()
  expect(
    await screen.findByRole('button', { name: 'alice' })
  ).toBeInTheDocument()
  expect(screen.getByText('45 / 100')).toBeInTheDocument()
  expect(screen.getByText('1,234')).toBeInTheDocument()
  expect(screen.getByText('Varied system prompts')).toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent(
    'Scores prioritize review. They do not prove redistribution or trigger bans.'
  )
})

test('opens per-key counts and restores keyboard focus when evidence closes', async () => {
  const user = userEvent.setup()
  await renderWatchlist()
  const evidence = await screen.findByRole('button', { name: 'View evidence' })
  await user.click(evidence)
  const dialog = await screen.findByRole('dialog', {
    name: 'Observation details',
  })
  expect(within(dialog).getByText('personal key')).toBeInTheDocument()
  expect(within(dialog).getByText('5 unique / 8 samples')).toBeInTheDocument()
  expect(within(dialog).getByText('6 unique / 8 samples')).toBeInTheDocument()
  expect(
    within(dialog).getByText('Top prompt share: 12.5%')
  ).toBeInTheDocument()
  expect(within(dialog).getByText('8.8.8.0/24')).toBeInTheDocument()
  expect(within(dialog).getByText('AS15169')).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  await waitFor(() => expect(evidence).toHaveFocus())
})

test('opens the selected account ban records from observation details', async () => {
  const user = userEvent.setup()
  const selected: number[] = []
  selectUser = (userId) => selected.push(userId)
  await renderWatchlist()
  await user.click(await screen.findByRole('button', { name: 'View evidence' }))
  await user.click(screen.getByRole('button', { name: 'View ban records' }))
  expect(selected).toEqual([42])
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})

test('distinguishes unavailable Redis observations from an empty active watchlist', async () => {
  await renderWatchlist({
    result: {
      ...RESULT,
      available: false,
      unavailable_reason: 'redis_disabled',
      items: [],
      total: 0,
    },
  })
  expect(
    await screen.findByText('Observations unavailable')
  ).toBeInTheDocument()
  expect(
    screen.getByText('Redis is required to collect distribution observations.')
  ).toBeInTheDocument()
  expect(
    screen.queryByText('No distribution observations found')
  ).not.toBeInTheDocument()
})

test('shows an empty result when there are no matching observations', async () => {
  await renderWatchlist({ result: { ...RESULT, items: [], total: 0 } })
  expect(
    await screen.findByText('No distribution observations found')
  ).toBeInTheDocument()
})

test('reports a failed business response and loads data after retry', async () => {
  let fail = true
  const user = userEvent.setup()
  await renderWatchlist({
    response: async () => {
      if (fail) {
        return {
          data: {
            success: false,
            message: 'Observation storage is unavailable',
          },
        }
      }
      return { data: { success: true, data: RESULT } }
    },
  })
  expect(
    await screen.findByText('Unable to load distribution observations')
  ).toBeInTheDocument()
  expect(
    screen.queryByText('No distribution observations found')
  ).not.toBeInTheDocument()
  fail = false
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('button', { name: 'alice' })
  ).toBeInTheDocument()
})

test('restores independent URL filters and sends the selected page to the server', async () => {
  const user = userEvent.setup()
  const router = await renderWatchlist({
    initialEntry:
      '/?tab=distribution&distPage=2&distPageSize=10&distFilter=alice&ipFilter=8.8.8.8',
    result: { ...RESULT, total: 45 },
  })
  await screen.findByRole('button', { name: 'alice' })
  const initialParams = new URL(requests.at(-1) ?? '', 'http://localhost')
    .searchParams
  expect(initialParams.get('p')).toBe('2')
  expect(initialParams.get('page_size')).toBe('10')
  expect(initialParams.get('filter')).toBe('alice')
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      distPage: 3,
      ipFilter: '8.8.8.8',
    })
  )
  await waitFor(() =>
    expect(
      new URL(requests.at(-1) ?? '', 'http://localhost').searchParams.get('p')
    ).toBe('3')
  )
})

test('marks incomplete evidence and keeps long key names readable in the dialog', async () => {
  const longName = 'public-key-name'.repeat(20)
  const token = {
    ...OBSERVATION.tokens[0],
    token_name: longName,
    prompt_limited: true,
  }
  const user = userEvent.setup()
  await renderWatchlist({
    result: {
      ...RESULT,
      items: [{ ...OBSERVATION, tokens: [token] }],
      scope_limited: true,
    },
  })
  expect(
    await screen.findByText(
      'The observation limit was reached. Some active keys are omitted from this ranking.'
    )
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'View evidence' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText(longName)).toHaveClass('break-all')
  expect(
    within(dialog).getByText(
      'Observation limits reached. Incomplete prompt or network signals are excluded from the score.'
    )
  ).toBeInTheDocument()
  expect(within(dialog).getAllByText('Partial observation')).toHaveLength(2)
})

test('updates translated labels and number formatting when the interface language changes', async () => {
  for (const [language, resource] of [
    ['zhCN', zh],
    ['zhTW', zhTW],
    ['fr', fr],
  ] as const) {
    i18next.addResourceBundle(language, 'translation', {
      'Redistribution watchlist':
        resource.translation['Redistribution watchlist'],
    })
  }
  await renderWatchlist()
  await screen.findByRole('button', { name: 'alice' })
  for (const [language, title] of [
    ['zhCN', '疑似分发观察榜'],
    ['zhTW', '疑似分發觀察榜'],
  ] as const) {
    await act(async () => {
      await i18next.changeLanguage(language)
    })
    expect(screen.getByText(title)).toBeInTheDocument()
    expect(screen.getByText('1,234')).toBeInTheDocument()
  }
  await act(async () => {
    await i18next.changeLanguage('fr')
  })
  expect(screen.getByText('Redistribution à examiner')).toBeInTheDocument()
  expect(screen.getByText(/1\s234/)).toBeInTheDocument()
})
