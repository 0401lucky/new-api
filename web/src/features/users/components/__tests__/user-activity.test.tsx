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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as UsersRoute } from '@/routes/_authenticated/users/index'
import { useAuthStore } from '@/stores/auth-store'

import { Users } from '../../index'
import type { GetUserActivityParams, User, UserActivityPage } from '../../types'

const clients: QueryClient[] = []
const fixtureUsers: User[] = [
  {
    id: 2,
    username: 'inactive-account',
    display_name: '',
    role: 1,
    status: 1,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    activity: 'very_inactive',
    last_request_at: 1700000000,
    no_request_days: 40,
    cleanup_eligible: true,
  },
  {
    id: 3,
    username: 'new-account',
    display_name: '',
    role: 1,
    status: 1,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    activity: 'never_requested',
    last_request_at: 0,
    no_request_days: 1,
    cleanup_eligible: false,
  },
  {
    id: 4,
    username: 'unused-old-account',
    display_name: '',
    role: 1,
    status: 1,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    activity: 'never_requested',
    last_request_at: 0,
    no_request_days: 45,
    cleanup_eligible: true,
  },
]

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

async function renderUserActivity(
  options: { initialEntry?: string; failure?: boolean; empty?: boolean } = {}
) {
  const state = {
    users: options.empty ? [] : [...fixtureUsers],
    failure: options.failure ?? false,
    cleanupFailure: false,
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (path, config) => {
    if (path === '/api/group/') {
      return { data: { success: true, data: ['default', 'premium'] } }
    }
    if (path === '/api/verify/methods') {
      return {
        data: {
          success: true,
          data: {
            scope: 'admin.user.batch_delete',
            methods: [{ method: '2fa', available: true }],
            oauth_providers: [],
            password_encryption_enabled: false,
          },
        },
      }
    }
    if (path === '/api/user/activity') {
      if (state.failure) throw new Error('Statistics are unavailable')
      const params = config?.params as GetUserActivityParams
      let items = state.users
      if (params.activity === 'cleanup') {
        items = items.filter((user) => user.cleanup_eligible)
      } else if (params.activity) {
        items = items.filter((user) => user.activity === params.activity)
      }
      const data: UserActivityPage = {
        items,
        total: items.length,
        page: params.p ?? 1,
        page_size: params.page_size ?? 20,
        summary: {
          total: state.users.length,
          active: 0,
          inactive: 0,
          very_inactive: 1,
          never_requested: 2,
          unknown: 0,
          cleanup_candidates: 2,
        },
        tracking_started_at: 1600000000,
        as_of: 1704000000,
      }
      return { data: { success: true, data } }
    }
    return {
      data: {
        success: true,
        data: { items: state.users, total: state.users.length },
      },
    }
  })
  const post = vi
    .spyOn(api, 'post')
    .mockImplementation(async (path, payload) => {
      if (path === '/api/verify') {
        return {
          data: {
            success: true,
            data: {
              proof_token: 'cleanup-proof',
              method: '2fa',
              scope: 'admin.user.batch_delete',
              expires_at: Math.floor(Date.now() / 1000) + 60,
            },
          },
        }
      }
      if (path === '/api/user/activity/batch-delete') {
        if (state.cleanupFailure) {
          return {
            data: {
              success: false,
              message:
                'Some selected users are no longer eligible for cleanup. Refresh the list and select again.',
            },
          }
        }
        const ids = (payload as { user_ids: number[] }).user_ids
        state.users = state.users.filter((user) => !ids.includes(user.id))
        return { data: { success: true, data: { deleted: ids.length } } }
      }
      throw new Error(`Unexpected request: ${String(path)}`)
    })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const users = createRoute({
    getParentRoute: () => auth,
    path: 'users/',
    validateSearch: UsersRoute.options.validateSearch,
    component: () => (
      <TooltipProvider>
        <Users />
      </TooltipProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([users])]),
    history: createMemoryHistory({
      initialEntries: [options.initialEntry ?? '/users/?tab=statistics'],
    }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { get, post, router, state }
}

it('opens statistics from the user tab and persists the selected activity filter in the URL', async () => {
  const user = userEvent.setup()
  const { get, router } = await renderUserActivity({ initialEntry: '/users/' })
  await screen.findByText('inactive-account')
  expect(get).not.toHaveBeenCalledWith('/api/user/activity', expect.anything())
  await user.click(screen.getByRole('tab', { name: 'User statistics' }))
  await screen.findByRole('button', { name: /^Very inactive users/ })
  await user.click(screen.getByRole('button', { name: /^Very inactive users/ }))
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/user/activity',
      expect.objectContaining({
        params: expect.objectContaining({ activity: 'very_inactive', p: 1 }),
      })
    )
  )
  expect(router.state.location.search).toMatchObject({
    tab: 'statistics',
    activity: 'very_inactive',
    page: 1,
  })
  expect(
    screen.getByRole('button', { name: /^Very inactive users/ })
  ).toHaveAttribute('aria-pressed', 'true')
  await waitFor(() =>
    expect(screen.queryByText('new-account')).not.toBeInTheDocument()
  )
  await user.click(screen.getByRole('button', { name: /^All users/ }))
  await screen.findByText('new-account')
  expect(screen.getByRole('tab', { name: 'User statistics' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

it('restores the cleanup filter from the URL and clears selection when the filter changes', async () => {
  const user = userEvent.setup()
  const { get } = await renderUserActivity({
    initialEntry: '/users/?tab=statistics&activity=cleanup',
  })
  await screen.findByText('inactive-account')
  expect(screen.queryByText('new-account')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledWith(
    '/api/user/activity',
    expect.objectContaining({
      params: expect.objectContaining({ activity: 'cleanup' }),
    })
  )
  await user.click(screen.getByRole('checkbox', { name: 'Select all' }))
  await screen.findByRole('button', { name: 'Delete selected users' })
  await user.click(screen.getByRole('button', { name: /^Never requested/ }))
  await screen.findByText('new-account')
  expect(
    screen.queryByRole('button', { name: 'Delete selected users' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('checkbox', { name: 'Select all' })).not.toBeChecked()
})

it('selects only eligible users, confirms the exact selection, and requires verification before deleting', async () => {
  const user = userEvent.setup()
  const { post } = await renderUserActivity()
  const newRow = await screen.findByRole('row', { name: /new-account/ })
  expect(within(newRow).getByRole('checkbox')).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  await user.click(within(newRow).getByRole('checkbox'))
  expect(within(newRow).getByRole('checkbox')).not.toBeChecked()
  await user.click(screen.getByRole('checkbox', { name: 'Select all' }))
  await user.click(
    await screen.findByRole('button', { name: 'Delete selected users' })
  )
  let confirmation = screen.getByRole('alertdialog')
  expect(within(confirmation).getByText(/inactive-account/)).toBeVisible()
  expect(within(confirmation).getByText(/unused-old-account/)).toBeVisible()
  expect(
    within(confirmation).queryByText(/new-account/)
  ).not.toBeInTheDocument()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(post).not.toHaveBeenCalled()

  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  confirmation = screen.getByRole('alertdialog')
  await user.click(
    within(confirmation).getByRole('button', { name: 'Delete selected users' })
  )
  await screen.findByLabelText('Authenticator code or backup code')
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalledWith(
    '/api/user/activity/batch-delete',
    expect.anything(),
    expect.anything()
  )
  await user.type(
    screen.getByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/activity/batch-delete',
      { user_ids: [2, 4] },
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'cleanup-proof' },
        singleUseAuthorization: true,
      })
    )
  )
  expect(post).toHaveBeenCalledWith(
    '/api/verify',
    expect.objectContaining({
      scope: 'admin.user.batch_delete',
      context: { user_ids: [2, 4] },
    }),
    expect.anything()
  )
  await waitFor(() =>
    expect(screen.queryByText('inactive-account')).not.toBeInTheDocument()
  )
  expect(screen.getByText('new-account')).toBeVisible()
})

it('shows a retryable error instead of an empty user list when statistics fail to load', async () => {
  const user = userEvent.setup()
  const { state } = await renderUserActivity({ failure: true })
  await screen.findByText('Failed to load user statistics')
  expect(screen.queryByText('No Users Found')).not.toBeInTheDocument()
  state.failure = false
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('inactive-account')
})

it('shows an empty state and disables select-all when no users match', async () => {
  await renderUserActivity({ empty: true })
  await screen.findByText('No Users Found')
  expect(screen.getByRole('checkbox', { name: 'Select all' })).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  expect(
    screen.queryByRole('button', { name: 'Delete selected users' })
  ).not.toBeInTheDocument()
})

it('keeps accounts and clears the stale selection when the server rejects cleanup', async () => {
  const user = userEvent.setup()
  const { state, post } = await renderUserActivity()
  state.cleanupFailure = true
  await screen.findByText('inactive-account')
  await user.click(screen.getByRole('checkbox', { name: 'Select all' }))
  await user.click(
    await screen.findByRole('button', { name: 'Delete selected users' })
  )
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Delete selected users',
    })
  )
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/activity/batch-delete',
      { user_ids: [2, 4] },
      expect.anything()
    )
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  await screen.findByText('inactive-account')
  expect(screen.getByText('unused-old-account')).toBeVisible()
  expect(screen.getByRole('checkbox', { name: 'Select all' })).not.toBeChecked()
})

it('offers selection and confirmation on mobile while keeping ineligible accounts disabled', async () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query.includes('max-width') || matchMedia(query).matches,
  }))
  const user = userEvent.setup()
  await renderUserActivity()
  await screen.findByText('inactive-account')
  expect(
    screen.getByRole('checkbox', { name: 'Select row 2' })
  ).toHaveAttribute('aria-disabled', 'true')
  await user.click(screen.getByRole('checkbox', { name: 'Select row 1' }))
  await user.click(screen.getByRole('checkbox', { name: 'Select row 3' }))
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  expect(
    within(screen.getByRole('alertdialog')).getByRole('list', {
      name: 'Selected users',
    })
  ).toHaveTextContent('inactive-account')
  expect(
    within(screen.getByRole('alertdialog')).queryByText(/new-account/)
  ).not.toBeInTheDocument()
})
