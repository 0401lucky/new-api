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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { OverviewDashboard } from '../overview-dashboard'

let client: QueryClient

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
})

beforeEach(() => {
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'dashboard-user',
    role: 1,
    quota: 1000000,
    used_quota: 1000,
    request_count: 1,
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/token/?p=1&size=10':
        return { data: { success: true, data: { items: [] } } }
      case '/api/status':
        return {
          data: {
            data: {
              api_info_enabled: false,
              announcements_enabled: true,
              announcements: [
                {
                  id: 1,
                  type: 'info',
                  content: 'Journal launch',
                  publishDate: '2026-10-18T00:00:00Z',
                },
              ],
              faq_enabled: false,
              uptime_kuma_enabled: false,
            },
          },
        }
      case '/api/user/models':
        return { data: { success: true, data: ['gpt-4o-mini'] } }
      case '/api/data/self':
        return { data: { success: true, data: [] } }
      default:
        throw new Error(`Unexpected dashboard request: ${url}`)
    }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  vi.restoreAllMocks()
})

async function renderOverview() {
  const router = createRouter({
    routeTree: createRootRoute({ component: OverviewDashboard }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('journal dashboard overview', () => {
  it('marks the page as immersive and keeps every decoration out of the accessibility tree', async () => {
    await renderOverview()

    const announcement = await screen.findByRole('button', {
      name: /Journal launch/,
    })
    expect(announcement).toBeVisible()
    expect(screen.getByText('Fresh notes')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
    expect(screen.getByText('Steady progress, keep going!')).toHaveAttribute(
      'aria-hidden',
      'true'
    )

    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('data-journal-level', 'immersive')
    expect(
      main.querySelector('[data-slot="overview-summary"]')
    ).toBeInTheDocument()
    expect(
      main.querySelector('[data-slot="overview-balance-slip"]')
    ).toBeInTheDocument()

    const decorations = main.querySelectorAll(
      '.journal-doodle, .journal-tape, .journal-paper-clip, .journal-hand, .journal-sticky-note'
    )
    expect(decorations.length).toBe(4)
    for (const selector of [
      '.journal-tape',
      '.journal-paper-clip',
      '.journal-hand',
      '.journal-sticky-note',
    ]) {
      expect(main.querySelector(selector)).not.toBeNull()
    }
    decorations.forEach((node) => {
      expect(node).toHaveAttribute('aria-hidden', 'true')
      expect(node).toHaveClass('pointer-events-none')
    })
  })

  it('keeps announcement notes clickable and opens the detail dialog', async () => {
    const user = userEvent.setup()
    await renderOverview()

    const announcement = await screen.findByRole('button', {
      name: /Journal launch/,
    })
    await user.click(announcement)

    expect(await screen.findByRole('dialog')).toBeVisible()
    expect(screen.getByText('Announcement Details')).toBeVisible()
  })
})
