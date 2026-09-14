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
import { waitFor } from '@testing-library/react'
import { AxiosError, isCancel, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { applyAuthBundle } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

import { donationApi, DonationRequestError } from '../api'
import {
  alice,
  aliceSession,
  batchFixture,
  bob,
  deferredDonationResponse,
  initializeDonationSession,
  response,
} from './fixtures'

const originalAdapter = api.defaults.adapter
afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

it('cancels a late response when the originating user changes and never sends the key as the new user', async () => {
  initializeDonationSession()
  const deferred = deferredDonationResponse()
  const requests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = (config) => {
    requests.push(config)
    return deferred.promise
  }
  const result = donationApi
    .submit(
      aliceSession,
      { campaign_id: 1, keys_text: 'synthetic-private-alice-key' },
      crypto.randomUUID()
    )
    .catch((error: unknown) => error)
  await waitFor(() => expect(requests).toHaveLength(1))
  useAuthStore.getState().auth.setBundle(bob)
  expect(requests[0]?.signal?.aborted).toBe(true)
  const request = requests[0]
  if (!request) throw new Error('Expected a captured request')
  deferred.resolve(response(request, batchFixture()))
  expect(isCancel(await result)).toBe(true)
  expect(request.headers.get('Authorization')).toBe(
    'Bearer synthetic-alice-access'
  )
  expect(requests).toHaveLength(1)
})

it('cancels a request when the same user switches login sessions', async () => {
  initializeDonationSession()
  const deferred = deferredDonationResponse()
  let request: InternalAxiosRequestConfig | undefined
  api.defaults.adapter = (config) => {
    request = config
    return deferred.promise
  }
  const result = donationApi
    .connection(aliceSession)
    .catch((error: unknown) => error)
  await waitFor(() => expect(request).toBeDefined())
  useAuthStore.getState().auth.setBundle({
    ...alice,
    session: { ...alice.session, sid: 'replacement-session' },
  })
  if (!request) throw new Error('Expected a captured request')
  deferred.resolve(response(request, {}))
  expect(isCancel(await result)).toBe(true)
})

it('pre-refreshes an expiring session and cancels the write if authentication changes before the refresh returns', async () => {
  initializeDonationSession({ ...alice, access_expires_at: 1 })
  const refresh = vi
    .spyOn(XMLHttpRequest.prototype, 'send')
    .mockImplementation(() => {})
  const send = vi.fn(async (config: InternalAxiosRequestConfig) =>
    response(config, batchFixture())
  )
  api.defaults.adapter = send
  const result = donationApi
    .submit(
      aliceSession,
      { campaign_id: 1, keys_text: 'synthetic-pre-refresh-key' },
      crypto.randomUUID()
    )
    .catch((error: unknown) => error)
  await waitFor(() => expect(refresh).toHaveBeenCalledOnce())
  applyAuthBundle(bob)
  const refreshXHR = refresh.mock.contexts[0]
  if (!(refreshXHR instanceof XMLHttpRequest)) {
    throw new Error('Expected a refresh request')
  }
  Object.defineProperties(refreshXHR, {
    status: { value: 200, configurable: true },
    statusText: { value: 'OK', configurable: true },
    readyState: { value: 4, configurable: true },
    responseText: {
      value: JSON.stringify({ success: true, data: alice }),
      configurable: true,
    },
  })
  refreshXHR.onloadend?.(new ProgressEvent('loadend'))
  expect(isCancel(await result)).toBe(true)
  expect(send).not.toHaveBeenCalled()
  expect(useAuthStore.getState().auth.user?.id).toBe(2)
})

it('does not refresh or replay a rejected credential write and strips Axios request data from the error', async () => {
  initializeDonationSession()
  const refresh = vi.spyOn(XMLHttpRequest.prototype, 'send')
  const send = vi.fn(async (config: InternalAxiosRequestConfig) => {
    throw new AxiosError(
      'synthetic-secret-token',
      'ERR_BAD_REQUEST',
      config,
      undefined,
      {
        ...response(config, {}),
        status: 401,
        data: { success: false, message: 'synthetic-secret-token' },
      }
    )
  })
  api.defaults.adapter = send
  const failure = await donationApi
    .saveConnection(aliceSession, {
      base_url: 'https://donation.example',
      token: 'synthetic-secret-token',
    })
    .catch((error: unknown) => error)
  expect(failure).toBeInstanceOf(DonationRequestError)
  expect(String(failure)).not.toContain('synthetic-secret-token')
  expect(JSON.stringify(failure)).not.toContain('synthetic-secret-token')
  expect(failure).not.toHaveProperty('config')
  expect(failure).not.toHaveProperty('cause')
  expect(refresh).not.toHaveBeenCalled()
  expect(send).toHaveBeenCalledTimes(1)
})
