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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { DonationSubmissionForm } from '../components/submission-form'
import {
  aliceSession,
  batchFixture,
  campaign,
  deferredDonationResponse,
  initializeDonationSession,
  response,
  unconfirmedBatchFixture,
} from './fixtures'

const originalAdapter = api.defaults.adapter

it('preserves the next unsent draft when a previously confirmed batch receives another GET update', async () => {
  const saved = unconfirmedBatchFixture()
  const confirmed = batchFixture()
  api.defaults.adapter = async (config) =>
    response(config, config.method === 'post' ? confirmed : [campaign])
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const props = {
    session: aliceSession,
    resume: saved,
    onResult: vi.fn(),
    onNew: () => {},
  }
  const view = render(<DonationSubmissionForm {...props} />, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        <TooltipProvider>{children}</TooltipProvider>
      </QueryClientProvider>
    ),
  })
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('API keys'), 'original-key')
  await user.click(
    screen.getByRole('button', { name: 'Retry original submission' })
  )
  await waitFor(() => expect(screen.getByLabelText('API keys')).toHaveValue(''))
  view.rerender(
    <DonationSubmissionForm
      {...props}
      resume={null}
      observedBatch={confirmed}
    />
  )
  await user.type(screen.getByLabelText('API keys'), 'next-unsent-private-key')
  view.rerender(
    <DonationSubmissionForm
      {...props}
      resume={null}
      observedBatch={{
        ...confirmed,
        updated_at_ms: confirmed.updated_at_ms + 1,
      }}
    />
  )
  expect(screen.getByLabelText('API keys')).toHaveValue(
    'next-unsent-private-key'
  )
  view.unmount()
  client.clear()
})
beforeEach(() => initializeDonationSession())
afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

it('keeps the input cleared and usable when a GET confirms before an old POST returns unconfirmed', async () => {
  const deferred = deferredDonationResponse()
  const saved = unconfirmedBatchFixture()
  let request: InternalAxiosRequestConfig | undefined
  api.defaults.adapter = async (config) => {
    if (config.method === 'post') {
      request = config
      return deferred.promise
    }
    return response(config, [campaign])
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onResult = vi.fn()
  const props = {
    session: aliceSession,
    resume: saved,
    onResult,
    onNew: () => {},
  }
  const view = render(<DonationSubmissionForm {...props} />, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        <TooltipProvider>{children}</TooltipProvider>
      </QueryClientProvider>
    ),
  })
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('API keys'), 'same-original-key')
  await user.click(
    screen.getByRole('button', { name: 'Retry original submission' })
  )
  await waitFor(() => expect(request).toBeDefined())
  view.rerender(
    <DonationSubmissionForm
      {...props}
      resume={null}
      observedBatch={batchFixture()}
    />
  )
  await waitFor(() => expect(screen.getByLabelText('API keys')).toHaveValue(''))
  if (!request) throw new Error('Expected request')
  deferred.resolve(response(request, saved))
  await waitFor(() => expect(screen.getByLabelText('API keys')).toBeEnabled())
  expect(screen.getByLabelText('API keys')).toHaveValue('')
  expect(onResult).not.toHaveBeenCalled()
  view.unmount()
  client.clear()
})

it.each(['accepted', 'error'] as const)(
  'ignores an old POST %s after the user opens another original submission',
  async (outcome) => {
    const deferred = deferredDonationResponse()
    const first = unconfirmedBatchFixture()
    const second = {
      ...first,
      id: '66666666-6666-4666-8666-666666666666',
      request_key: '77777777-7777-4777-8777-777777777777',
      campaign_id: 2,
      campaign_name: 'Other pending campaign',
    }
    let request: InternalAxiosRequestConfig | undefined
    api.defaults.adapter = async (config) => {
      if (config.method === 'post') {
        request = config
        const result = await deferred.promise
        if (outcome === 'error') {
          throw new AxiosError(
            'old failure',
            'ERR_BAD_REQUEST',
            config,
            undefined,
            {
              ...result,
              status: 400,
              data: { success: false, code: 'DONATION_INVALID_REQUEST' },
            }
          )
        }
        return result
      }
      return response(config, [
        campaign,
        { ...campaign, id: 2, name: second.campaign_name },
      ])
    }
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const onResult = vi.fn()
    const props = {
      session: aliceSession,
      resume: first,
      onResult,
      onNew: () => {},
    }
    const view = render(<DonationSubmissionForm {...props} />, {
      wrapper: ({ children }) => (
        <QueryClientProvider client={client}>
          <TooltipProvider>{children}</TooltipProvider>
        </QueryClientProvider>
      ),
    })
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('API keys'), 'first-original-key')
    await user.click(
      screen.getByRole('button', { name: 'Retry original submission' })
    )
    await waitFor(() => expect(request).toBeDefined())
    view.rerender(<DonationSubmissionForm {...props} resume={second} />)
    if (!request) throw new Error('Expected request')
    deferred.resolve(response(request, batchFixture()))
    await waitFor(() => expect(screen.getByLabelText('API keys')).toBeEnabled())
    expect(onResult).not.toHaveBeenCalled()
    expect(
      screen.queryByText(
        'The donation settings or input are invalid. Check the fields and try again.'
      )
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Donation campaign' })
    ).toHaveValue(second.campaign_name)
    view.unmount()
    client.clear()
  }
)
