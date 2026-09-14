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
  createRootRouteWithContext,
  createRoute,
  createRouter,
  RouterProvider,
  type Register,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import { TooltipProvider } from '@/components/ui/tooltip'
import {
  initializeDonationSession,
  response,
} from '@/features/donations/__tests__/fixtures'
import {
  parseHeaderNavModules as parseSettings,
  serializeHeaderNavModules,
} from '@/features/system-settings/maintenance/config'
import { api } from '@/lib/api'
import { parseHeaderNavModules } from '@/lib/nav-modules'
import { Route as RootRoute } from '@/routes/__root'
import { Route as AuthenticatedRoute } from '@/routes/_authenticated/route'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from '../components/app-header'
import { PublicHeader } from '../components/public-header'

const originalAdapter = api.defaults.adapter
const clients: QueryClient[] = []
let visibilityStyle: HTMLStyleElement

beforeEach(() => {
  localStorage.clear()
  initializeDonationSession()
  // Model the mobile utility visibility contract; real viewport layout is also
  // exercised by the application's browser acceptance checks.
  visibilityStyle = document.createElement('style')
  visibilityStyle.textContent = '.hidden { display: none; }'
  document.head.append(visibilityStyle)
  api.defaults.adapter = async (config) =>
    response(
      config,
      config.url === '/api/status'
        ? {
            system_name: 'new-api',
            quota_per_unit: 100,
            HeaderNavModules: '{}',
          }
        : ''
    )
})
afterEach(() => {
  cleanup()
  visibilityStyle.remove()
  for (const client of clients.splice(0)) client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  vi.restoreAllMocks()
})

async function renderHeader(ui: ReactNode, protectedDonation = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const rootGuard = RootRoute.options.beforeLoad
  if (typeof rootGuard !== 'function') {
    throw new Error('Expected the application bootstrap guard')
  }
  const root = createRootRouteWithContext<{ queryClient: QueryClient }>()({
    beforeLoad: rootGuard,
    component: () => ui,
  })
  const home = createRoute({ getParentRoute: () => root, path: '/' })
  const guard = AuthenticatedRoute.options.beforeLoad
  const authenticated = createRoute<
    Register,
    typeof root,
    '/',
    '/',
    '_authenticated'
  >({
    getParentRoute: () => root,
    id: '_authenticated',
    beforeLoad: async (context) => {
      if (protectedDonation && typeof guard === 'function') {
        await guard(context)
      }
    },
  })
  const donation = createRoute({
    getParentRoute: () => authenticated,
    path: '/donations',
  })
  const signIn = createRoute({ getParentRoute: () => root, path: '/sign-in' })
  const router = createRouter({
    context: { queryClient: client },
    routeTree: root.addChildren([
      home,
      authenticated.addChildren([donation]),
      signIn,
    ]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
    defaultPendingMinMs: 0,
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <SidebarProvider>
          <RouterProvider router={router} />
        </SidebarProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  return router
}

it('keeps donations enabled for legacy navigation settings and preserves an explicit disabled value', () => {
  expect(parseHeaderNavModules('{}').donations).toBe(true)
  expect(parseHeaderNavModules('{"donations":false}').donations).toBe(false)
  const settings = parseSettings('{"home":false}')
  expect(settings.donations).toBe(true)
  expect(
    parseSettings(serializeHeaderNavModules({ ...settings, donations: false }))
      .donations
  ).toBe(false)
})

it('keeps the app navigation trigger visible on mobile and exposes the donation entry', async () => {
  await renderHeader(
    <AppHeader
      showSearch={false}
      showNotifications={false}
      showConfigDrawer={false}
      showProfileDropdown={false}
    />
  )
  const user = userEvent.setup()
  const trigger = await screen.findByRole('button', {
    name: 'Toggle navigation menu',
  })
  expect(trigger).toBeVisible()
  await user.click(trigger)
  const menu = await screen.findByRole('menu')
  expect(
    within(menu).getByRole('menuitem', { name: 'Donations' })
  ).toBeVisible()
})

it('opens the public mobile donation link and retains it as the existing login return destination', async () => {
  useAuthStore.getState().auth.reset('complete')
  vi.spyOn(XMLHttpRequest.prototype, 'send').mockImplementation(
    function (this: XMLHttpRequest) {
      Object.defineProperties(this, {
        status: { value: 401, configurable: true },
        statusText: { value: 'Unauthorized', configurable: true },
        responseText: {
          value: JSON.stringify({ success: false }),
          configurable: true,
        },
        readyState: { value: 4, configurable: true },
      })
      this.onloadend?.(new ProgressEvent('loadend'))
    }
  )
  const router = await renderHeader(
    <PublicHeader
      showAuthButtons={false}
      showNotifications={false}
      showLanguageSwitcher={false}
      showThemeSwitch={false}
    />,
    true
  )
  const user = userEvent.setup()
  const trigger = await screen.findByRole('button', {
    name: 'Toggle navigation menu',
  })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await user.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await user.click(await screen.findByRole('link', { name: 'Donations' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
  expect(router.state.location.search).toMatchObject({ redirect: '/donations' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
