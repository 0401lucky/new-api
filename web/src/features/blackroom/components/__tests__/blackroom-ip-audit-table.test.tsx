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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { PageFooterProvider } from '@/components/layout/components/page-footer'
import { api } from '@/lib/api'

import { blackroomSearchSchema } from '../../lib/blackroom-search'
import type { BlackroomIPAuditItem } from '../../types'
import { BlackroomIPAuditTable } from '../blackroom-ip-audit-table'
import { BlackroomProvider } from '../blackroom-provider'

const clients: QueryClient[] = []
const auditRequests: string[] = []
const footers: HTMLElement[] = []
let selectUser: (userId: number) => void = () => undefined

const SHARED_IP: BlackroomIPAuditItem = {
  ip: '203.0.113.7',
  request_count: 940,
  user_count: 6,
  first_seen_at: 1_754_800_000,
  last_seen_at: 1_754_900_000,
  users: [
    { user_id: 42, username: 'alice', request_count: 640 },
    { user_id: 43, username: 'bob', request_count: 300 },
  ],
  users_truncated: true,
}

function BlackroomHarness() {
  const search = auditRoute.useSearch()
  const navigate = auditRoute.useNavigate()

  return (
    <BlackroomProvider>
      <BlackroomIPAuditTable
        search={search}
        navigate={navigate}
        onSelectUser={selectUser}
      />
    </BlackroomProvider>
  )
}

const rootRoute = createRootRoute()
const auditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  validateSearch: blackroomSearchSchema,
  component: BlackroomHarness,
})

function lastAuditParams(): URLSearchParams {
  const url = auditRequests.at(-1)
  if (!url) throw new Error('no IP audit request was issued')
  return new URL(url, 'http://localhost').searchParams
}

async function renderAuditTable(options: {
  initialEntry: string
  items?: BlackroomIPAuditItem[]
  total?: number
}) {
  const items = options.items ?? [SHARED_IP]
  const total = options.total ?? items.length
  auditRequests.length = 0

  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url.startsWith('/api/blackroom/ip-audit')) auditRequests.push(url)
    return {
      data: {
        success: true,
        data: {
          start_at: 0,
          end_at: 0,
          total,
          page: 1,
          page_size: 20,
          items,
        },
      },
    }
  })

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)

  // 分页走 PageFooterPortal，测试里给它一个挂在 body 上的容器，
  // screen 才能查询到分页按钮。
  const footer = document.createElement('div')
  document.body.append(footer)
  footers.push(footer)

  const router = createRouter({
    routeTree: rootRoute.addChildren([auditRoute]),
    history: createMemoryHistory({ initialEntries: [options.initialEntry] }),
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
  selectUser = () => undefined
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  footers.splice(0).forEach((footer) => footer.remove())
})

test('renders one row per IP with its user and request counts', async () => {
  await renderAuditTable({ initialEntry: '/' })

  expect(await screen.findByText('203.0.113.7')).toBeInTheDocument()
  expect(screen.getByText('940')).toBeInTheDocument()
  expect(screen.getByText('6')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'alice' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'bob' })).toBeInTheDocument()
})

test('reports how many related users are hidden beyond the first five', async () => {
  await renderAuditTable({ initialEntry: '/' })

  expect(await screen.findByText('+4 more')).toBeInTheDocument()
})

test('opens the selected user ban records when a related user is clicked', async () => {
  const user = userEvent.setup()
  const selected: number[] = []
  selectUser = (userId) => selected.push(userId)
  await renderAuditTable({ initialEntry: '/' })

  await user.click(await screen.findByRole('button', { name: 'bob' }))

  expect(selected).toEqual([43])
})

test('shows the empty state when no IP was observed', async () => {
  await renderAuditTable({ initialEntry: '/', items: [] })

  expect(
    await screen.findByText('No IP observations found')
  ).toBeInTheDocument()
})

test('restores the IP audit filters and page from the URL', async () => {
  const router = await renderAuditTable({
    initialEntry:
      '/?tab=ip-audit&ipPage=2&ipPageSize=10&ipFilter=203.0.113.7&ipStart=1754800000&ipEnd=1755000000',
    total: 45,
  })

  await screen.findByText('203.0.113.7')
  expect(router.state.location.search).toMatchObject({ ipPage: 2 })

  const params = lastAuditParams()
  expect(params.get('p')).toBe('2')
  expect(params.get('page_size')).toBe('10')
  expect(params.get('filter')).toBe('203.0.113.7')
  expect(params.get('start_at')).toBe('1754800000')
  expect(params.get('end_at')).toBe('1755000000')
})

test('falls back to defaults instead of crashing on a hand-edited URL', async () => {
  await renderAuditTable({
    initialEntry: '/?ipPage=abc&ipPageSize=-5&ipStart=oops&ipEnd=1e999',
  })

  expect(await screen.findByText('203.0.113.7')).toBeInTheDocument()

  const params = lastAuditParams()
  expect(params.get('p')).toBe('1')
  expect(params.get('page_size')).toBe('20')
  expect(params.has('filter')).toBe(false)
  expect(params.has('start_at')).toBe(false)
  expect(params.has('end_at')).toBe(false)
})

test('pushes a history entry when the user goes to another page', async () => {
  const user = userEvent.setup()
  const router = await renderAuditTable({ initialEntry: '/', total: 45 })
  await screen.findByText('203.0.113.7')
  const historyLengthBefore = router.history.length
  expect(router.history.canGoBack()).toBe(false)

  await user.click(screen.getByRole('button', { name: 'Go to next page' }))

  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ ipPage: 2 })
  )
  expect(router.history.length).toBe(historyLengthBefore + 1)
  expect(router.history.canGoBack()).toBe(true)
  expect(lastAuditParams().get('p')).toBe('2')
})

test('replaces the current history entry when the keyword filter changes', async () => {
  const user = userEvent.setup()
  const router = await renderAuditTable({ initialEntry: '/' })
  await screen.findByText('203.0.113.7')
  const historyLengthBefore = router.history.length

  await user.type(screen.getByPlaceholderText('Search IP address...'), '203')

  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({ ipFilter: '203' })
  )
  expect(router.history.length).toBe(historyLengthBefore)
  expect(lastAuditParams().get('filter')).toBe('203')
})

test('reset clears the keyword and the date range in one history entry', async () => {
  const user = userEvent.setup()
  const router = await renderAuditTable({
    initialEntry: '/?ipFilter=203&ipStart=1754800000&ipPage=2',
  })
  await screen.findByText('203.0.113.7')
  const historyLengthBefore = router.history.length

  await user.click(screen.getByRole('button', { name: 'Reset' }))

  await waitFor(() =>
    expect(router.state.location.search).not.toMatchObject({ ipFilter: '203' })
  )
  expect(router.state.location.search).not.toHaveProperty('ipStart')
  expect(router.state.location.search).not.toHaveProperty('ipPage')
  expect(router.history.length).toBe(historyLengthBefore)
})
