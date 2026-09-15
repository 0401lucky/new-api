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
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  AxiosError,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, it } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { donationBatchIsProcessing } from '../api'
import { Donations } from '../index'
import type { DonationBatchDetail } from '../types'
import {
  batchFixture,
  batchWithItems,
  bob,
  campaign,
  deferredDonationResponse,
  initializeDonationSession,
  pendingReviewItem,
  renderDonation,
  response,
  unconfirmedBatchFixture,
} from './fixtures'

const originalAdapter = api.defaults.adapter
beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  initializeDonationSession()
})
afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  sessionStorage.clear()
})

function submissionNetwork(props: {
  current: () => DonationBatchDetail | null
  post: (
    config: InternalAxiosRequestConfig
  ) => AxiosResponse | Promise<AxiosResponse>
  closed?: boolean
}) {
  api.defaults.adapter = async (config) => {
    const url = config.url ?? ''
    if (url === '/api/donations/campaigns') {
      return response(config, [
        { ...campaign, enabled: !props.closed, available: !props.closed },
      ])
    }
    if (url === '/api/donations/batches' && config.method === 'post') {
      return props.post(config)
    }
    if (url === '/api/donations/batches') {
      const batch =
        config.headers.get('Authorization') === `Bearer ${bob.access_token}`
          ? null
          : props.current()
      return response(config, {
        page: 1,
        page_size: 10,
        total: batch ? 1 : 0,
        items: batch ? [batch] : [],
      })
    }
    if (url.includes('/api/donations/batches/')) {
      return response(config, props.current())
    }
    throw new Error(`Unexpected test endpoint: ${url}`)
  }
}

async function chooseCampaign(user: ReturnType<typeof userEvent.setup>) {
  const input = await screen.findByRole('combobox', {
    name: 'Donation campaign',
  })
  await waitFor(() => expect(input).toBeEnabled())
  await user.click(input)
  await user.click(
    await screen.findByRole('option', { name: /Community keys/ })
  )
}

it('shows mixed outcomes and credits only accepted keys while clearing raw input and mutation variables', async () => {
  let current: DonationBatchDetail | null = null
  const accepted = batchFixture()
  const first = accepted.items[0]
  if (!first) throw new Error('Expected fixture item')
  const mixed = batchFixture({
    items: [
      first,
      {
        ...first,
        id: '44444444-4444-4444-8444-444444444444',
        line: 3,
        state: 'invalid',
        key_mask: '••••',
        reason_code: 'invalid_credential',
        credential_id: null,
        accepted_at_ms: null,
        reward_state: 'none',
        rewarded_quota: 0,
        rewarded_at_ms: null,
      },
      {
        ...first,
        id: '55555555-5555-4555-8555-555555555555',
        line: 4,
        state: 'duplicate',
        reason_code: 'duplicate_item',
        credential_id: null,
        accepted_at_ms: null,
        reward_state: 'none',
        rewarded_quota: 0,
        rewarded_at_ms: null,
      },
    ],
    summary: {
      total: 3,
      accepted: 1,
      invalid: 1,
      duplicate: 1,
      processing: 0,
      rewarded: 1,
      rewarded_quota: 25,
    },
  })
  submissionNetwork({
    current: () => current,
    post: (config) => {
      current = mixed
      return response(config, mixed)
    },
  })
  const view = await renderDonation(<Donations />, true)
  const user = userEvent.setup()
  await chooseCampaign(user)
  const keys =
    'synthetic-private-key-0001\n\ninvalid-private-key\nsynthetic-private-key-0001'
  await user.click(screen.getByLabelText('API keys'))
  await user.paste(keys)
  await user.click(screen.getByRole('button', { name: 'Donate keys' }))
  const result = await screen.findByRole('region', {
    name: 'Submission results',
  })
  expect(within(result).getByText('Accepted')).toBeVisible()
  expect(within(result).getByText('Invalid')).toBeVisible()
  expect(within(result).getByText('Duplicate key')).toBeVisible()
  expect(within(result).getByText('Permanent quota: $0.25')).toBeVisible()
  expect(within(result).getByText('3', { selector: 'td' })).toBeVisible()
  expect(screen.getByLabelText('API keys')).toHaveValue('')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(
    JSON.stringify(
      view.client
        .getMutationCache()
        .getAll()
        .map((entry) => entry.state)
    )
  ).not.toContain('synthetic-private-key')
  expect(JSON.stringify(localStorage)).not.toContain('synthetic-private-key')
  expect(JSON.stringify(sessionStorage)).not.toContain('synthetic-private-key')
  view.unmount()
  view.client.clear()
})

it('replays an unconfirmed submission with the original request ID instead of creating another donation', async () => {
  const accepted = batchFixture()
  const initialItem = accepted.items[0]
  if (!initialItem) throw new Error('Expected fixture item')
  const pending = batchFixture({
    reception_state: 'unconfirmed',
    items: [
      {
        ...initialItem,
        state: 'unconfirmed',
        reward_state: 'none',
        rewarded_quota: 0,
        rewarded_at_ms: null,
        credential_id: null,
        accepted_at_ms: null,
      },
    ],
    summary: {
      total: 1,
      accepted: 0,
      invalid: 0,
      duplicate: 0,
      processing: 1,
      rewarded: 0,
      rewarded_quota: 0,
    },
  })
  const requests: InternalAxiosRequestConfig[] = []
  let current: DonationBatchDetail | null = null
  submissionNetwork({
    current: () => current,
    post: (config) => {
      requests.push(config)
      const result = requests.length === 1 ? pending : accepted
      current = {
        ...result,
        request_key: String(config.headers.get('Idempotency-Key')),
      }
      return response(config, current)
    },
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await chooseCampaign(user)
  await user.type(
    screen.getByLabelText('API keys'),
    'synthetic-unconfirmed-key'
  )
  await user.click(screen.getByRole('button', { name: 'Donate keys' }))
  await waitFor(() => expect(screen.getByLabelText('API keys')).toBeDisabled())
  expect(screen.getByLabelText('API keys')).toHaveValue(
    'synthetic-unconfirmed-key'
  )
  await user.click(
    await screen.findByRole('button', { name: 'Retry original submission' })
  )
  await waitFor(() => expect(screen.getByLabelText('API keys')).toHaveValue(''))
  expect(requests).toHaveLength(2)
  expect(requests[0]?.headers.get('Idempotency-Key')).toEqual(
    requests[1]?.headers.get('Idempotency-Key')
  )
  expect(requests[0]?.data).toEqual(requests[1]?.data)
  view.unmount()
  view.client.clear()
})

it('clears retained key text once a later receipt query confirms the submission', async () => {
  const accepted = batchFixture()
  let current: DonationBatchDetail | null = null
  submissionNetwork({
    current: () => current,
    post: (config) => {
      current = {
        ...unconfirmedBatchFixture(),
        request_key: String(config.headers.get('Idempotency-Key')),
      }
      return response(config, current)
    },
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await chooseCampaign(user)
  await user.type(
    screen.getByLabelText('API keys'),
    'retained-until-confirmed-key'
  )
  await user.click(screen.getByRole('button', { name: 'Donate keys' }))
  await waitFor(() => expect(screen.getByLabelText('API keys')).toBeDisabled())
  if (!current) throw new Error('Expected a saved request')
  current = {
    ...accepted,
    request_key: (current as DonationBatchDetail).request_key,
  }
  await user.click(
    within(
      screen.getByRole('region', { name: 'Submission results' })
    ).getByRole('button', { name: 'Refresh' })
  )
  await waitFor(() => expect(screen.getByLabelText('API keys')).toHaveValue(''))
  expect(screen.getByLabelText('API keys')).toBeEnabled()
  view.unmount()
  view.client.clear()
})

it('resumes an unconfirmed history entry using its frozen campaign and original request ID after the campaign closes', async () => {
  const saved = unconfirmedBatchFixture()
  let current: DonationBatchDetail | null = saved
  let posted: InternalAxiosRequestConfig | undefined
  submissionNetwork({
    closed: true,
    current: () => current,
    post: (config) => {
      posted = config
      current = batchFixture()
      return response(config, current)
    },
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'View results' }))
  await user.click(
    await screen.findByRole('button', { name: 'Resume submission' })
  )
  await user.click(screen.getByLabelText('API keys'))
  await user.paste('\noriginal-private-key')
  await user.click(
    screen.getByRole('button', { name: 'Retry original submission' })
  )
  await waitFor(() => expect(posted).toBeDefined())
  expect(posted?.headers.get('Idempotency-Key')).toBe(saved.request_key)
  expect(JSON.parse(String(posted?.data))).toEqual({
    campaign_id: saved.campaign_id,
    keys_text: '\noriginal-private-key',
  })
  view.unmount()
  view.client.clear()
})

it('keeps pending retry action identity after a lost response and never includes key text in retry requests', async () => {
  const first = batchFixture().items[0]
  if (!first) throw new Error('Expected fixture item')
  let current = batchFixture({
    items: [
      {
        ...first,
        state: 'retry_pending',
        retryable: true,
        reason_code: 'retry_exhausted',
        reward_state: 'none',
        credential_id: null,
        accepted_at_ms: null,
        rewarded_quota: 0,
        rewarded_at_ms: null,
      },
    ],
    summary: {
      total: 1,
      accepted: 0,
      invalid: 0,
      duplicate: 0,
      processing: 1,
      rewarded: 0,
      rewarded_quota: 0,
    },
  })
  const actions: InternalAxiosRequestConfig[] = []
  submissionNetwork({
    current: () => current,
    post: (config) => response(config, current),
  })
  const base = api.defaults.adapter
  api.defaults.adapter = async (config) => {
    if (config.url?.endsWith('/retry')) {
      actions.push(config)
      if (actions.length === 1) {
        throw new AxiosError('lost retry response', 'ERR_NETWORK', config)
      }
      current = batchFixture()
      return response(config, current)
    }
    if (typeof base !== 'function') throw new Error('Expected adapter function')
    return base(config)
  }
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'View results' }))
  await user.click(
    await screen.findByRole('button', { name: 'Retry pending keys' })
  )
  await waitFor(() => expect(actions).toHaveLength(1))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Retry pending keys' })
    ).toBeEnabled()
  )
  await user.click(screen.getByRole('button', { name: 'Retry pending keys' }))
  await waitFor(() => expect(actions).toHaveLength(2))
  expect(actions[0]?.headers.get('Idempotency-Key')).toEqual(
    actions[1]?.headers.get('Idempotency-Key')
  )
  expect(actions.map((request) => request.data)).toEqual(['{}', '{}'])
  view.unmount()
  view.client.clear()
})

it('clears the old account input immediately and discards its late submission result after switching accounts', async () => {
  let current: DonationBatchDetail | null = null
  const pending = deferredDonationResponse()
  let request: InternalAxiosRequestConfig | undefined
  submissionNetwork({
    current: () => current,
    post: (config) => {
      request = config
      return pending.promise
    },
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await chooseCampaign(user)
  await user.type(
    screen.getByLabelText('API keys'),
    'alice-private-pending-key'
  )
  await user.click(screen.getByRole('button', { name: 'Donate keys' }))
  await waitFor(() => expect(request).toBeDefined())
  act(() => useAuthStore.getState().auth.setBundle(bob))
  expect(screen.getByLabelText('API keys')).toHaveValue('')
  if (!request) throw new Error('Expected captured request')
  expect(request.signal?.aborted).toBe(true)
  current = batchFixture()
  const captured = request
  await act(async () => {
    pending.resolve(response(captured, current))
  })
  expect(
    screen.queryByRole('region', { name: 'Submission results' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Reward credited')).not.toBeInTheDocument()
  expect(useAuthStore.getState().auth.user?.id).toBe(bob.user.id)
  view.unmount()
  view.client.clear()
})

it('separates keys awaiting manual review from keys still handled automatically', async () => {
  const reviewing = pendingReviewItem()
  const validating = {
    ...pendingReviewItem({
      id: '77777777-7777-4777-8777-777777777777',
      line: 2,
    }),
    state: 'validating' as const,
    effective_mode: 'auto' as const,
    staging_expires_at_ms: null,
  }
  const rejected = {
    ...pendingReviewItem({
      id: '88888888-8888-4888-8888-888888888888',
      line: 3,
    }),
    state: 'rejected' as const,
    reason_code: 'review_rejected',
    review_state: 'rejected' as const,
    review_note: 'The key stopped working before the review.',
  }
  const batch = batchWithItems([reviewing, validating, rejected])
  batch.summary.processing = 1
  submissionNetwork({
    current: () => batch,
    post: (config) => response(config, batch),
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'View results' }))
  const result = await screen.findByRole('region', {
    name: 'Submission results',
  })
  expect(within(result).getByText('Pending review')).toBeVisible()
  expect(within(result).getByText('Validating')).toBeVisible()
  expect(within(result).getByText('Review rejected')).toBeVisible()
  expect(within(result).getByText(/Temporary storage expires/)).toBeVisible()
  expect(
    within(result).getByText('The key stopped working before the review.')
  ).toBeVisible()
  expect(
    within(result).queryByText(
      'Processing is temporarily unavailable. Refresh or retry later.'
    )
  ).not.toBeInTheDocument()
  expect(
    within(result).getByText(
      '1 keys are waiting for manual review. Rewards are credited only after an administrator approves the keys and they are received.'
    )
  ).toBeVisible()
  expect(
    within(result).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  expect(
    within(result).queryByRole('button', { name: 'Reject' })
  ).not.toBeInTheDocument()
  expect(
    within(result).queryByRole('button', { name: 'Run test' })
  ).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('only keeps fast polling alive while keys are still handled automatically', () => {
  expect(donationBatchIsProcessing(batchWithItems([pendingReviewItem()]))).toBe(
    false
  )
  expect(
    donationBatchIsProcessing(
      batchWithItems([pendingReviewItem({ state: 'rejected' })])
    )
  ).toBe(false)
  expect(
    donationBatchIsProcessing(
      batchWithItems([pendingReviewItem({ state: 'validating' })])
    )
  ).toBe(true)
  expect(
    donationBatchIsProcessing(
      batchWithItems([
        pendingReviewItem(),
        pendingReviewItem({ state: 'accepted', reward_state: 'pending' }),
      ])
    )
  ).toBe(true)
  expect(
    donationBatchIsProcessing(
      batchWithItems([
        pendingReviewItem({ state: 'rejected', effective_mode: 'auto' }),
      ])
    )
  ).toBe(false)
})

it('describes expired temporary storage without labeling the donated key invalid', async () => {
  const expired = pendingReviewItem({
    state: 'invalid',
    reason_code: 'staging_expired',
    review_state: 'expired',
  })
  const batch = batchWithItems([expired])
  submissionNetwork({
    current: () => batch,
    post: (config) => response(config, batch),
  })
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'View results' }))
  const result = await screen.findByRole('region', {
    name: 'Submission results',
  })
  expect(within(result).getByText('Temporary storage expired')).toBeVisible()
  expect(within(result).queryByText('Invalid')).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('warns the donor that a manual campaign is reviewed before the reward is credited', async () => {
  api.defaults.adapter = async (config) => {
    const url = config.url ?? ''
    if (url === '/api/donations/campaigns') {
      return response(config, [
        { ...campaign, validation_mode: 'manual_review' },
      ])
    }
    if (url === '/api/donations/batches') {
      return response(config, { page: 1, page_size: 10, total: 0, items: [] })
    }
    throw new Error(`Unexpected test endpoint: ${url}`)
  }
  const view = await renderDonation(<Donations />)
  const user = userEvent.setup()
  await chooseCampaign(user)
  expect(
    await screen.findByText(
      'An administrator reviews each key manually. The reward is credited only after the review passes and the key is received.'
    )
  ).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('shows the exact Chinese risk notice before submission without a consent dialog or checkbox', async () => {
  const translations = createInstance()
  await translations
    .use(initReactI18next)
    .init({ lng: 'zh', resources: { zh }, initAsync: false })
  submissionNetwork({
    current: () => null,
    post: (config) => response(config, batchFixture()),
  })
  const view = await renderDonation(
    <I18nextProvider i18n={translations}>
      <Donations />
    </I18nextProvider>
  )
  const notice = screen.getByText(
    '请勿使用主账号的 API key。捐献后的使用可能因平台风控或其他原因导致账号受限、封禁，或 key 失效，请确认能承担相关风险。'
  )
  const submit = screen.getByRole('button', {
    name: translations.t('Donate keys'),
  })
  expect(notice).toBeVisible()
  expect(
    notice.compareDocumentPosition(submit) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})
