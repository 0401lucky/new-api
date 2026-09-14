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
import { render } from '@testing-library/react'
import {
  AxiosHeaders,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { type ReactNode, StrictMode } from 'react'
import { vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type {
  DonationBatchDetail,
  DonationCampaign,
  DonationConnection,
  DonationSession,
  ManagedCampaign,
} from '../types'

export const alice: AuthBundle = {
  access_token: 'synthetic-alice-access',
  token_type: 'Bearer',
  access_expires_at: 2_500_000_000,
  user: { id: 1, username: 'alice', role: 1, status: 1 },
  session: {
    sid: 'alice-session',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 2_500_000_000,
  },
}
export const bob: AuthBundle = {
  ...alice,
  access_token: 'synthetic-bob-access',
  user: { id: 2, username: 'bob', role: 1, status: 1 },
  session: { ...alice.session, sid: 'bob-session' },
}
export const admin: AuthBundle = {
  ...alice,
  user: { ...alice.user, role: 100 },
}
export const aliceSession: DonationSession = {
  userId: 1,
  sid: 'alice-session',
  key: '1:alice-session',
}
export const campaign: DonationCampaign = {
  id: 1,
  name: 'Community keys',
  description: 'Community contribution',
  reward_quota: 25,
  enabled: true,
  available: true,
  unavailable_reason: '',
}
export const connection: DonationConnection = {
  base_url: 'https://donation.example',
  configured: true,
  instance_id: 'instance-uuid',
  source_id: 'source-uuid',
  version: 1,
  updated_at_ms: 1000,
}
export const managedCampaign: ManagedCampaign = {
  ...campaign,
  version: 1,
  instance_id: 'instance-uuid',
  group_id: 7,
  group_name: 'Gemini community',
  target_revision: 'revision',
  created_at_ms: 1000,
  updated_at_ms: 1000,
}
export const group = {
  id: 7,
  name: 'Gemini community',
  channel_id: 'gemini',
  connection_type: 'api_key',
  enabled: true,
  can_probe: true,
  target_revision: 'revision',
  unavailable_reason: '',
}
const originalMatchMedia = window.matchMedia.bind(window)

export function batchFixture(
  overrides: Partial<DonationBatchDetail> = {}
): DonationBatchDetail {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    request_key: '22222222-2222-4222-8222-222222222222',
    user_id: 1,
    username: 'alice',
    linux_do_id: '',
    campaign_id: 1,
    campaign_version: 1,
    campaign_name: campaign.name,
    instance_id: 'instance-uuid',
    group_id: 7,
    group_name: group.name,
    reward_quota: 25,
    reception_state: 'confirmed',
    last_error: '',
    created_at_ms: 1789300000000,
    updated_at_ms: 1789300001000,
    items: [
      {
        id: '33333333-3333-4333-8333-333333333333',
        batch_id: '11111111-1111-4111-8111-111111111111',
        line: 1,
        key_mask: '••••0001',
        state: 'accepted',
        reason_code: '',
        retryable: false,
        credential_id: 100,
        accepted_at_ms: 1789300001000,
        reward_state: 'rewarded',
        reward_reason: '',
        rewarded_quota: 25,
        rewarded_at_ms: 1789300001000,
        created_at_ms: 1789300000000,
        updated_at_ms: 1789300001000,
      },
    ],
    summary: {
      total: 1,
      accepted: 1,
      invalid: 0,
      duplicate: 0,
      processing: 0,
      rewarded: 1,
      rewarded_quota: 25,
    },
    ...overrides,
  }
}

export function response<T>(
  config: InternalAxiosRequestConfig,
  data: T
): AxiosResponse {
  return {
    data: { success: true, message: '', data },
    status: 200,
    statusText: 'OK',
    headers: new AxiosHeaders(),
    config,
  }
}

export function deferredDonationResponse() {
  let complete: (value: AxiosResponse) => void = () => {
    throw new Error('Response gate was not initialized')
  }
  const promise = new Promise<AxiosResponse>((resolve) => {
    complete = resolve
  })
  return { promise, resolve: complete }
}

export function unconfirmedBatchFixture(): DonationBatchDetail {
  const original = batchFixture()
  return {
    ...original,
    reception_state: 'unconfirmed',
    items: original.items.map((item) => ({
      ...item,
      state: 'unconfirmed',
      credential_id: null,
      accepted_at_ms: null,
      reward_state: 'none',
      rewarded_quota: 0,
      rewarded_at_ms: null,
    })),
    summary: {
      total: 1,
      accepted: 0,
      invalid: 0,
      duplicate: 0,
      processing: 1,
      rewarded: 0,
      rewarded_quota: 0,
    },
  }
}

export function initializeDonationSession(bundle = alice): void {
  useAuthStore.getState().auth.setBundle(bundle)
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 100 } })
  useSystemConfigStore.getState().setLoading(false)
}

export async function renderDonation(ui: ReactNode, strict = false) {
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => {
    const min = /min-width:\s*(\d+)px/.exec(query)
    const max = /max-width:\s*(\d+)px/.exec(query)
    return {
      ...originalMatchMedia(query),
      matches: min
        ? 1280 >= Number(min[1])
        : Boolean(max && 1280 <= Number(max[1])),
    }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/donations',
    component: () => ui,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/donations'] }),
    defaultPendingMinMs: 0,
  })
  await router.load()
  const element = (
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  const rendered = render(strict ? <StrictMode>{element}</StrictMode> : element)
  return { ...rendered, client, router }
}
