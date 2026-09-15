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
import i18n from 'i18next'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { api } from '@/lib/api'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { donationRecordIsProcessing } from '../api'
import { DonationRecords } from '../records'
import type {
  DonationRecord,
  DonationReviewAction,
  DonationReviewContext,
  DonationTestMetadata,
} from '../types'
import {
  admin,
  batchWithItems,
  bob,
  initializeDonationSession,
  pendingReviewItem,
  readOnlyRecordsAdmin,
  renderDonation,
  response,
  reviewContextFixture,
  deferredDonationResponse,
  streamingTestBody,
  successfulTestResult,
} from './fixtures'

const originalAdapter = api.defaults.adapter

function pendingRecord(
  itemOverrides: Parameters<typeof pendingReviewItem>[0] = {}
): DonationRecord {
  const item = pendingReviewItem(itemOverrides)
  return { item, batch: batchWithItems([item]), reward: null, events: [] }
}

function appliedAction(
  kind: DonationReviewAction['kind'] = 'approve'
): DonationReviewAction {
  const record = pendingRecord()
  return {
    action_id: 'action-uuid',
    batch_id: record.batch.id,
    item_id: record.item.id,
    actor_id: 42,
    kind,
    expected_item_revision: 3,
    review_target_revision: reviewContextFixture().review_target_revision,
    status: 'applied',
    reason_code: '',
    effect_revision: 4,
    note: '',
    applied_at_ms: 1789500000000,
    created_at_ms: 1789499999000,
  }
}

function reviewNetwork(options: {
  record: () => DonationRecord
  context?: () => DonationReviewContext
  actions?: InternalAxiosRequestConfig[]
  actionReads?: InternalAxiosRequestConfig[]
  tests?: InternalAxiosRequestConfig[]
  action?: (config: InternalAxiosRequestConfig) => AxiosResponse
  actionRead?: () => DonationReviewAction
  test?: (config: InternalAxiosRequestConfig) => AxiosResponse
  testRead?: (config: InternalAxiosRequestConfig) => DonationTestMetadata
}) {
  api.defaults.adapter = async (config) => {
    const url = config.url ?? ''
    if (url.endsWith('/review-context')) {
      return response(config, (options.context ?? reviewContextFixture)())
    }
    if (url.endsWith('/review-actions')) {
      options.actions?.push(config)
      if (options.action) return options.action(config)
      const kind = String(config.data).includes('"reject"')
        ? 'reject'
        : 'approve'
      return response(config, appliedAction(kind))
    }
    if (url.includes('/review-actions/')) {
      options.actionReads?.push(config)
      return response(
        config,
        options.actionRead ? options.actionRead() : appliedAction()
      )
    }
    if (url.endsWith('/tests')) {
      options.tests?.push(config)
      return (
        options.test?.(config) ??
        response(
          config,
          successfulTestResult({
            test_id: String(config.headers.get('Idempotency-Key')),
          })
        )
      )
    }
    if (url.includes('/tests/')) {
      return response(
        config,
        options.testRead?.(config) ??
          successfulTestResult({ test_id: url.split('/').at(-1) })
      )
    }
    if (url === '/api/donations/admin/records') {
      return response(config, {
        page: 1,
        page_size: 20,
        total: 1,
        items: [options.record()],
      })
    }
    if (url.includes('/api/donations/admin/records/')) {
      return response(config, options.record())
    }
    throw new Error(`Unexpected test endpoint: ${url}`)
  }
}

async function openDetails(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'Details' }))
  return screen.findByRole('dialog', { name: 'Donation details' })
}

async function reviewSection() {
  const dialog = await screen.findByRole('dialog', { name: 'Donation details' })
  return within(dialog).findByRole('region', { name: 'Manual review' })
}

interface FakeStreamRequest {
  payload: unknown
  headers: Record<string, string>
  aborted: boolean
}

let streamBehavior: ((xhr: FakeStreamXhr) => void) | null = null

class FakeStreamXhr extends EventTarget {
  static HEADERS_RECEIVED = 2
  responseText = ''
  status = 200
  readyState = 1
  response = ''
  withCredentials = false
  payload: unknown = null
  headers: Record<string, string> = {}
  aborted = false
  contentType = 'text/event-stream'
  open(): void {}
  setRequestHeader(name: string, value: string): void {
    this.headers[name] = value
  }
  getAllResponseHeaders(): string {
    return `content-type: ${this.contentType}`
  }
  abort(): void {
    this.aborted = true
    this.readyState = 4
    this.dispatchEvent(new Event('abort'))
  }
  send(payload?: unknown): void {
    this.payload = payload
    streamBehavior?.(this)
  }
}

/** Replaces the browser transport that sse.js drives, so the real receiver
 * event framing (`meta` / `delta` / `done`) is exercised without a network. */
function installFakeStream(behavior: (xhr: FakeStreamRequest) => void) {
  const original = window.XMLHttpRequest
  const requests: FakeStreamRequest[] = []
  streamBehavior = (xhr) => {
    requests.push(xhr)
    behavior(xhr)
  }
  window.XMLHttpRequest = FakeStreamXhr as unknown as typeof XMLHttpRequest
  return {
    requests,
    restore() {
      window.XMLHttpRequest = original
      streamBehavior = null
    },
  }
}

function pushStreamBody(
  xhr: FakeStreamRequest,
  body: string,
  contentType = 'text/event-stream',
  complete = true
) {
  const stream = xhr as FakeStreamXhr
  stream.contentType = contentType
  stream.readyState = 2
  stream.dispatchEvent(new Event('readystatechange'))
  stream.responseText = body.replaceAll(
    'test-uuid',
    xhr.headers['Idempotency-Key'] ?? ''
  )
  stream.dispatchEvent(new ProgressEvent('progress'))
  if (complete) {
    stream.readyState = 4
    stream.dispatchEvent(new ProgressEvent('load'))
  }
}

beforeEach(() => {
  localStorage.clear()
  initializeDonationSession(admin)
})

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

it('keeps review and test actions hidden from a records administrator without the write permissions', async () => {
  initializeDonationSession(readOnlyRecordsAdmin)
  const actions: InternalAxiosRequestConfig[] = []
  const tests: InternalAxiosRequestConfig[] = []
  reviewNetwork({
    record: () =>
      pendingRecord({
        effective_mode: 'manual_review',
        review_action_id: 'action-uuid',
      }),
    actions,
    tests,
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  const dialog = await openDetails(user)
  const section = await reviewSection()
  expect(
    within(section).getByRole('heading', { name: 'Manual review' })
  ).toBeVisible()
  expect(within(section).getByText('Administrator #42')).toBeVisible()
  expect(
    within(section).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  expect(
    within(section).queryByRole('button', { name: 'Reject' })
  ).not.toBeInTheDocument()
  expect(
    within(section).queryByRole('button', { name: 'Move to manual review' })
  ).not.toBeInTheDocument()
  expect(
    within(dialog).queryByRole('region', { name: 'Model test' })
  ).not.toBeInTheDocument()
  expect(actions).toHaveLength(0)
  expect(tests).toHaveLength(0)
  view.unmount()
  view.client.clear()
})

it('keeps an uncertain review bound to the same action and checks its result without another write', async () => {
  const actions: InternalAxiosRequestConfig[] = []
  const actionReads: InternalAxiosRequestConfig[] = []
  const record = pendingRecord()
  reviewNetwork({
    record: () => record,
    actions,
    actionReads,
    action: (config) => {
      throw new AxiosError('Response lost', 'ERR_NETWORK', config)
    },
    actionRead: () => ({
      ...appliedAction('approve'),
      action_id: String(actions[0]?.headers.get('Idempotency-Key')),
    }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Approve' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Approve this donation',
  })
  await user.type(
    within(confirm).getByLabelText('Note (optional)'),
    'verified before connection loss'
  )
  await user.click(within(confirm).getByRole('button', { name: 'Approve' }))
  const check = await within(section).findByRole('button', {
    name: 'Check review result',
  })
  expect(
    within(section).queryByRole('button', { name: 'Reject' })
  ).not.toBeInTheDocument()
  await user.click(check)
  await waitFor(() => expect(actionReads).toHaveLength(1))
  expect(actionReads[0]?.url).toContain(
    String(actions[0]?.headers.get('Idempotency-Key'))
  )
  expect(actions).toHaveLength(1)
  view.unmount()
  view.client.clear()
})

it('disables approval when its UTF-8 note exceeds the receiver limit', async () => {
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({
    record: () => pendingRecord(),
    actions,
    context: () =>
      reviewContextFixture({
        review_limits: {
          ...reviewContextFixture().review_limits,
          max_note_bytes: 8,
        },
      }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Approve' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Approve this donation',
  })
  await user.type(within(confirm).getByLabelText('Note (optional)'), '人工核实')
  expect(
    within(confirm).getByRole('button', { name: 'Approve' })
  ).toBeDisabled()
  expect(within(confirm).getByRole('alert')).toHaveTextContent(
    'Please shorten this note.'
  )
  expect(actions).toHaveLength(0)
  view.unmount()
  view.client.clear()
})

it('replays the same immutable review intent when its original request never reached the server', async () => {
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({
    record: () => pendingRecord(),
    actions,
    action: (config) => {
      if (actions.length === 1) {
        throw new AxiosError('Request lost', 'ERR_NETWORK', config)
      }
      return response(config, {
        ...appliedAction('approve'),
        action_id: config.headers.get('Idempotency-Key'),
      })
    },
    actionRead: () => {
      const first = actions[0]
      if (!first) throw new Error('Expected the first review request')
      throw new AxiosError('Not found', 'ERR_BAD_REQUEST', first, undefined, {
        ...response(first, null),
        status: 404,
      })
    },
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Approve' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Approve this donation',
  })
  await user.type(
    within(confirm).getByLabelText('Note (optional)'),
    'Original review evidence'
  )
  await user.click(within(confirm).getByRole('button', { name: 'Approve' }))
  await user.click(
    await within(section).findByRole('button', { name: 'Check review result' })
  )
  await waitFor(() => expect(actions).toHaveLength(2))
  expect(actions[1]?.headers.get('Idempotency-Key')).toBe(
    actions[0]?.headers.get('Idempotency-Key')
  )
  expect(actions[1]?.data).toBe(actions[0]?.data)
  expect(JSON.parse(String(actions[1]?.data))).toMatchObject({
    kind: 'approve',
    note: 'Original review evidence',
    expected_item_revision: 3,
  })
  view.unmount()
  view.client.clear()
})

it('shows cancellation for a stopped call, cancels its transport, and ignores a late success', async () => {
  const pending = deferredDonationResponse()
  const tests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = (config) => {
    if (config.url?.endsWith('/tests')) {
      tests.push(config)
      return pending.promise
    }
    const url = config.url ?? ''
    if (url.endsWith('/review-context')) {
      return Promise.resolve(response(config, reviewContextFixture()))
    }
    if (url === '/api/donations/admin/records') {
      return Promise.resolve(
        response(config, {
          items: [pendingRecord()],
          total: 1,
          page: 1,
          page_size: 20,
        })
      )
    }
    return Promise.resolve(response(config, pendingRecord()))
  }
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const panel = await screen.findByRole('region', { name: 'Model test' })
  await user.type(
    within(panel).getByLabelText('User prompt'),
    'A call to cancel'
  )
  await user.click(
    within(panel).getByRole('switch', { name: 'Stream the response' })
  )
  await user.click(within(panel).getByRole('button', { name: 'Run test' }))
  await waitFor(() => expect(tests).toHaveLength(1))
  await user.click(within(panel).getByRole('button', { name: 'Stop test' }))
  expect(tests[0]?.signal?.aborted).toBe(true)
  expect(within(panel).getByText('Test cancelled')).toBeVisible()
  const request = tests[0]
  if (!request) throw new Error('Expected the running test request')
  await act(async () => {
    pending.resolve(response(request, successfulTestResult()))
  })
  expect(within(panel).queryByText('Test succeeded')).not.toBeInTheDocument()
  expect(tests).toHaveLength(1)
  view.unmount()
  view.client.clear()
})

it('shows the review decision, operator, decision time and the donor-visible reason', async () => {
  reviewNetwork({
    record: () =>
      pendingRecord({
        state: 'rejected',
        review_state: 'rejected',
        review_action_id: 'action-uuid',
        review_note: 'The key was already donated by another account.',
        reviewed_at_ms: 1789500000000,
        staging_expires_at_ms: 1789900000000,
      }),
    actionRead: () => appliedAction('reject'),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  const dialog = await openDetails(user)
  const section = await reviewSection()
  expect(within(section).getByText('Rejected')).toBeVisible()
  expect(within(section).getByText('Administrator #42')).toBeVisible()
  expect(
    within(section).getByText(
      formatTimestampToDate(1789500000000, 'milliseconds')
    )
  ).toBeVisible()
  expect(
    within(section).getByText('The key was already donated by another account.')
  ).toBeVisible()
  expect(
    within(section).getByText('The receiving target is unchanged.')
  ).toBeVisible()
  expect(
    within(section).getByText('No test has been run for this record.')
  ).toBeVisible()
  expect(within(dialog).getByText('Review rejected')).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('shows recent review commands and test metadata to a read-only administrator without retaining response text', async () => {
  initializeDonationSession(readOnlyRecordsAdmin)
  const test = successfulTestResult()
  delete test.text
  reviewNetwork({
    record: () => ({
      ...pendingRecord(),
      recent_review_actions: [
        {
          ...appliedAction('approve'),
          action_id: 'latest-action',
          status: 'pending',
          applied_at_ms: null,
          note: '<script>plain review note</script>',
        },
        {
          ...appliedAction('reject'),
          action_id: 'earlier-action',
          status: 'rejected',
          reason_code: 'already_reviewed',
        },
      ],
      recent_tests: [
        { ...test, test_id: 'latest-test', actor_id: 42 },
        {
          ...test,
          test_id: 'earlier-test',
          actor_id: 13,
          model: 'earlier-model',
          state: 'interrupted',
        },
      ],
    }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const reviews = await screen.findByRole('table', {
    name: 'Recent review actions',
  })
  expect(within(reviews).getByText('Awaiting confirmation')).toBeVisible()
  expect(within(reviews).getByText('Action not applied')).toBeVisible()
  expect(
    within(reviews).getByText('<script>plain review note</script>')
  ).toBeVisible()
  expect(reviews.querySelector('script')).toBeNull()
  const tests = screen.getByRole('table', { name: 'Recent model tests' })
  expect(within(tests).getByText('Test succeeded')).toBeVisible()
  expect(within(tests).getByText('Test interrupted')).toBeVisible()
  expect(within(tests).getByText('earlier-model')).toBeVisible()
  expect(within(tests).getByText('Administrator #13')).toBeVisible()
  expect(
    screen.queryByText('Hello from the donation key')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('states that the receiving target changed instead of pretending the record is reviewable', async () => {
  reviewNetwork({
    record: () => pendingRecord(),
    context: () =>
      reviewContextFixture({
        can_review: false,
        can_test: false,
        unavailable_reason: 'target_changed',
      }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  expect(
    await within(section).findByText(
      'The receiving target changed after this key was submitted. This record cannot be tested or approved.'
    )
  ).toBeVisible()
  expect(
    within(section).getByText(
      'The receiving target changed after this key was submitted.'
    )
  ).toBeVisible()
  expect(
    within(section).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  expect(within(section).getByRole('button', { name: 'Reject' })).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('approves a record after showing the donor, key mask and permanent reward', async () => {
  const record = pendingRecord()
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({ record: () => record, actions })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Approve' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Approve this donation',
  })
  expect(within(confirm).getByText('alice · 1')).toBeVisible()
  expect(within(confirm).getByText(record.item.key_mask)).toBeVisible()
  expect(within(confirm).getByText('$0.25')).toBeVisible()
  await user.type(
    within(confirm).getByLabelText('Note (optional)'),
    'checked manually'
  )
  await user.click(within(confirm).getByRole('button', { name: 'Approve' }))
  await waitFor(() => expect(actions).toHaveLength(1))
  const config = actions[0]
  if (!config) throw new Error('Expected a review action request')
  expect(JSON.parse(String(config.data))).toMatchObject({
    kind: 'approve',
    expected_item_revision: 3,
    review_target_revision: reviewContextFixture().review_target_revision,
    note: 'checked manually',
  })
  expect(JSON.parse(String(config.data))).not.toHaveProperty('action_id')
  expect(config.headers.get('Idempotency-Key')).toMatch(/^[a-f0-9-]{36}$/)
  expect(config.method).toBe('post')
  view.unmount()
  view.client.clear()
})

it('requires a donor-visible reason before a rejection can be confirmed', async () => {
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({ record: () => pendingRecord(), actions })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Reject' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Reject this donation',
  })
  const submit = within(confirm).getByRole('button', { name: 'Reject' })
  expect(submit).toBeDisabled()
  await user.type(
    within(confirm).getByLabelText('Reason the donor can read'),
    'The key stopped working before the review.'
  )
  expect(submit).toBeEnabled()
  await user.click(submit)
  await waitFor(() => expect(actions).toHaveLength(1))
  expect(JSON.parse(String(actions[0]?.data))).toMatchObject({
    kind: 'reject',
    note: 'The key stopped working before the review.',
  })
  view.unmount()
  view.client.clear()
})

it('reports a command rejection instead of pretending the record was reviewed', async () => {
  reviewNetwork({
    record: () => pendingRecord(),
    action: (config) =>
      response(config, {
        ...appliedAction('approve'),
        status: 'rejected',
        reason_code: 'already_reviewed',
      }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  await user.click(within(section).getByRole('button', { name: 'Approve' }))
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Approve this donation',
  })
  await user.click(within(confirm).getByRole('button', { name: 'Approve' }))
  expect(
    await within(section).findByText(
      'Another administrator already reviewed this record.'
    )
  ).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('offers the manual review entry only for exhausted automatic validation with staging left', async () => {
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({
    actions,
    record: () =>
      pendingRecord({
        state: 'retry_pending',
        reason_code: 'retry_exhausted',
        retryable: true,
        effective_mode: 'auto',
      }),
    context: () =>
      reviewContextFixture({
        effective_mode: 'auto',
        state: 'retry_pending',
        can_test: false,
        can_reject: false,
        review_action: 'enter_review',
      }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  expect(
    await within(section).findByRole('button', {
      name: 'Move to manual review',
    })
  ).toBeVisible()
  expect(
    within(section).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  expect(
    within(section).queryByRole('button', { name: 'Reject' })
  ).not.toBeInTheDocument()
  await user.click(
    within(section).getByRole('button', { name: 'Move to manual review' })
  )
  const confirm = await screen.findByRole('alertdialog', {
    name: 'Move to manual review',
  })
  await user.click(
    within(confirm).getByRole('button', { name: 'Move to manual review' })
  )
  await waitFor(() => expect(actions).toHaveLength(1))
  expect(JSON.parse(String(actions[0]?.data))).toMatchObject({
    kind: 'enter_review',
    expected_item_revision: 3,
    review_target_revision: reviewContextFixture().review_target_revision,
  })
  view.unmount()
  view.client.clear()
})

it('runs a non-streaming test for the open record without approving it', async () => {
  const tests: InternalAxiosRequestConfig[] = []
  const actions: InternalAxiosRequestConfig[] = []
  reviewNetwork({ record: () => pendingRecord(), tests, actions })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  const dialog = await openDetails(user)
  const panel = await within(dialog).findByRole('region', {
    name: 'Model test',
  })
  await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
  await user.click(
    within(panel).getByRole('switch', { name: 'Stream the response' })
  )
  await user.click(within(panel).getByRole('button', { name: 'Run test' }))
  await waitFor(() => expect(tests).toHaveLength(1))
  const config = tests[0]
  if (!config) throw new Error('Expected a test request')
  expect(JSON.parse(String(config.data))).toMatchObject({
    model: 'z-ai/glm-5.3-flash',
    prompt: 'Say hello',
    stream: false,
    expected_item_revision: 3,
    review_target_revision: reviewContextFixture().review_target_revision,
  })
  expect(JSON.parse(String(config.data))).not.toHaveProperty('test_id')
  expect(config.headers.get('Idempotency-Key')).toMatch(/^[a-f0-9-]{36}$/)
  expect(
    await within(panel).findByText('Hello from the donation key')
  ).toBeVisible()
  expect(within(panel).getByText('Test succeeded')).toBeVisible()
  const section = await reviewSection()
  expect(within(section).getAllByText('Test succeeded').length).toBeGreaterThan(
    0
  )
  expect(actions).toHaveLength(0)
  expect(within(section).getByText('Pending review')).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('streams the test response and clears it when the record is closed', async () => {
  const stream = installFakeStream((xhr) =>
    pushStreamBody(xhr, streamingTestBody())
  )
  try {
    reviewNetwork({ record: () => pendingRecord() })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    const dialog = await openDetails(user)
    const panel = await within(dialog).findByRole('region', {
      name: 'Model test',
    })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    expect(stream.requests).toHaveLength(1)
    expect(stream.requests[0]?.headers.Accept).toBe('text/event-stream')
    expect(JSON.parse(String(stream.requests[0]?.payload))).toMatchObject({
      expected_item_revision: 3,
      review_target_revision: reviewContextFixture().review_target_revision,
    })
    expect(JSON.parse(String(stream.requests[0]?.payload))).not.toHaveProperty(
      'test_id'
    )
    expect(
      await within(panel).findByText('Hello from the donation key')
    ).toBeVisible()
    expect(within(panel).getByText('Test succeeded')).toBeVisible()
    expect(stream.requests[0]?.aborted).toBe(true)
    expect(
      JSON.stringify(
        view.client
          .getQueryCache()
          .getAll()
          .map((query) => query.state)
      )
    ).not.toContain('Hello from the donation key')
    expect(
      JSON.stringify(
        view.client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state)
      )
    ).not.toContain('Say hello')
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Donation details' })
      ).not.toBeInTheDocument()
    )
    expect(
      screen.queryByText('Hello from the donation key')
    ).not.toBeInTheDocument()
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it('restores a pending review and its latest test without offering a new decision or replaying a call', async () => {
  const pending = {
    ...appliedAction('approve'),
    status: 'pending' as const,
    applied_at_ms: null,
  }
  const latest = successfulTestResult()
  delete latest.text
  const record = {
    ...pendingRecord(),
    pending_review_action: pending,
    latest_test: latest,
  }
  const actions: InternalAxiosRequestConfig[] = []
  const tests: InternalAxiosRequestConfig[] = []
  const reads: InternalAxiosRequestConfig[] = []
  reviewNetwork({
    record: () => record,
    actions,
    tests,
    actionReads: reads,
    actionRead: () => pending,
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  expect(within(section).getByText('Test succeeded')).toBeVisible()
  expect(
    within(section).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  expect(
    within(section).queryByRole('button', { name: 'Reject' })
  ).not.toBeInTheDocument()
  expect(
    within(section).getByRole('button', { name: 'Run test' })
  ).toBeDisabled()
  await user.click(
    within(section).getByRole('button', { name: 'Check review result' })
  )
  await waitFor(() => expect(reads).toHaveLength(1))
  expect(
    within(section).getByText(
      'The review result is not confirmed. Check the original action before making another decision.'
    )
  ).toBeVisible()
  expect(actions).toHaveLength(0)
  expect(tests).toHaveLength(0)
  expect(
    within(section).queryByText('Hello from the donation key')
  ).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('polls active decisions and tests while leaving a record waiting only for human review idle', () => {
  const record = pendingRecord()
  expect(donationRecordIsProcessing(record)).toBe(false)
  expect(
    donationRecordIsProcessing({
      ...record,
      pending_review_action: { ...appliedAction(), status: 'pending' },
    })
  ).toBe(true)
  expect(
    donationRecordIsProcessing({
      ...record,
      latest_test: successfulTestResult({
        state: 'running',
        finished_at_ms: null,
      }),
    })
  ).toBe(true)
  expect(
    donationRecordIsProcessing({
      ...record,
      latest_test: successfulTestResult(),
    })
  ).toBe(false)
})

it('clears an uncertain review after refreshed history confirms that the original command did not apply', async () => {
  const pending = {
    ...appliedAction('approve'),
    status: 'pending' as const,
    applied_at_ms: null,
  }
  let record: DonationRecord = {
    ...pendingRecord(),
    pending_review_action: pending,
    recent_review_actions: [pending],
  }
  reviewNetwork({ record: () => record })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  expect(
    within(section).getByRole('button', { name: 'Check review result' })
  ).toBeVisible()
  record = {
    ...record,
    pending_review_action: null,
    recent_review_actions: [
      { ...pending, status: 'rejected', reason_code: 'already_reviewed' },
    ],
  }
  await act(async () => {
    await view.client.invalidateQueries({
      queryKey: ['donations', '1:alice-session', 'record', record.item.id],
    })
  })
  await waitFor(() =>
    expect(
      within(section).queryByRole('button', { name: 'Check review result' })
    ).not.toBeInTheDocument()
  )
  expect(
    within(section).getByText(
      'Another administrator already reviewed this record.'
    )
  ).toBeVisible()
  view.unmount()
  view.client.clear()
})

it('stops a stream after partial text and keeps the response explicitly cancelled', async () => {
  const partial = streamingTestBody().split('event: done')[0] ?? ''
  const stream = installFakeStream((xhr) =>
    pushStreamBody(xhr, partial, 'text/event-stream', false)
  )
  try {
    reviewNetwork({ record: () => pendingRecord() })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    await openDetails(user)
    const panel = await screen.findByRole('region', { name: 'Model test' })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    await within(panel).findByText('Hello from the donation key')
    expect(
      screen.queryByRole('button', { name: 'Approve' })
    ).not.toBeInTheDocument()
    await user.click(within(panel).getByRole('button', { name: 'Stop test' }))
    expect(within(panel).getByText('Test cancelled')).toBeVisible()
    expect(within(panel).getByText('Hello from the donation key')).toBeVisible()
    expect(stream.requests[0]?.aborted).toBe(true)
    expect(stream.requests).toHaveLength(1)
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it('stops before displaying response text beyond the advertised byte limit', async () => {
  const stream = installFakeStream((xhr) =>
    pushStreamBody(xhr, streamingTestBody())
  )
  try {
    reviewNetwork({
      record: () => pendingRecord(),
      context: () =>
        reviewContextFixture({
          review_limits: {
            ...reviewContextFixture().review_limits,
            max_response_bytes: 8,
          },
        }),
    })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    await openDetails(user)
    const panel = await screen.findByRole('region', { name: 'Model test' })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    expect(
      await within(panel).findByText(
        'The test response exceeded the supported size limit.'
      )
    ).toBeVisible()
    expect(
      within(panel).queryByText('Hello from the donation key')
    ).not.toBeInTheDocument()
    expect(within(panel).queryByText('Test succeeded')).not.toBeInTheDocument()
    expect(stream.requests[0]?.aborted).toBe(true)
    expect(stream.requests).toHaveLength(1)
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it('shows a saved JSON result returned to a stream request without replaying the model call', async () => {
  const stream = installFakeStream((xhr) =>
    pushStreamBody(
      xhr,
      JSON.stringify({ success: true, data: successfulTestResult() }),
      'application/json'
    )
  )
  try {
    reviewNetwork({
      record: () => pendingRecord(),
      testRead: (config) => {
        const result = successfulTestResult({
          test_id: config.url?.split('/').at(-1),
          stream: true,
        })
        delete result.text
        return result
      },
    })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    await openDetails(user)
    const panel = await screen.findByRole('region', { name: 'Model test' })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    expect(await within(panel).findByText('Test succeeded')).toBeVisible()
    expect(
      within(panel).getByText(
        'The saved test result is available, but its response text is not retained.'
      )
    ).toBeVisible()
    expect(
      within(panel).queryByText('Hello from the donation key')
    ).not.toBeInTheDocument()
    expect(stream.requests).toHaveLength(1)
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it.each([
  {
    condition: 'closes after partial output without a terminal result',
    body: () => streamingTestBody().split('event: done')[0] ?? '',
    error: 'The model test stream ended before it reported a result.',
  },
  {
    condition: 'claims success before identifying the tested record',
    body: () =>
      `event: done${streamingTestBody().split('event: done')[1] ?? ''}`,
    error: 'The test result could not be read.',
  },
  {
    condition: 'identifies a different donation item',
    body: () =>
      streamingTestBody().replace(
        pendingReviewItem().id,
        '99999999-9999-4999-8999-999999999999'
      ),
    error: 'The test result could not be read.',
  },
])(
  'does not report success or reconnect when a stream $condition',
  async (entry) => {
    const stream = installFakeStream((xhr) => pushStreamBody(xhr, entry.body()))
    try {
      reviewNetwork({ record: () => pendingRecord() })
      const view = await renderDonation(<DonationRecords />)
      const user = userEvent.setup()
      await openDetails(user)
      const panel = await screen.findByRole('region', { name: 'Model test' })
      await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
      await user.click(within(panel).getByRole('button', { name: 'Run test' }))
      expect(await within(panel).findByText(entry.error)).toBeVisible()
      expect(
        within(panel).queryByText('Test succeeded')
      ).not.toBeInTheDocument()
      expect(stream.requests).toHaveLength(1)
      view.unmount()
      view.client.clear()
    } finally {
      stream.restore()
    }
  }
)

it('marks a historical test as out of date when the receiving target changed', async () => {
  const latest = successfulTestResult()
  delete latest.text
  reviewNetwork({
    record: () => ({ ...pendingRecord(), latest_test: latest }),
    context: () =>
      reviewContextFixture({
        can_review: false,
        can_test: false,
        can_reject: true,
        review_action: '',
        unavailable_reason: 'target_changed',
      }),
  })
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  await openDetails(user)
  const section = await reviewSection()
  expect(
    await within(section).findByText(
      'This test used an earlier receiving target and is no longer current.'
    )
  ).toBeVisible()
  expect(within(section).getByRole('button', { name: 'Reject' })).toBeVisible()
  expect(
    within(section).queryByRole('button', { name: 'Approve' })
  ).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('never shows a response that arrives after the record was closed', async () => {
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  const stream = installFakeStream((xhr) => {
    void gate.then(() => pushStreamBody(xhr, streamingTestBody()))
  })
  try {
    reviewNetwork({ record: () => pendingRecord() })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    const dialog = await openDetails(user)
    const panel = await within(dialog).findByRole('region', {
      name: 'Model test',
    })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Donation details' })
      ).not.toBeInTheDocument()
    )
    await act(async () => {
      release()
      await gate
    })
    expect(
      screen.queryByText('Hello from the donation key')
    ).not.toBeInTheDocument()
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it('aborts a running test when the signed-in account changes', async () => {
  const stream = installFakeStream(() => {})
  try {
    reviewNetwork({ record: () => pendingRecord() })
    const view = await renderDonation(<DonationRecords />)
    const user = userEvent.setup()
    const dialog = await openDetails(user)
    const panel = await within(dialog).findByRole('region', {
      name: 'Model test',
    })
    await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
    await user.click(within(panel).getByRole('button', { name: 'Run test' }))
    await within(panel).findByRole('button', { name: 'Stop test' })
    act(() => useAuthStore.getState().auth.setBundle(bob))
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'Donation details' })
      ).not.toBeInTheDocument()
    )
    expect(stream.requests[0]?.aborted).toBe(true)
    expect(
      screen.queryByText('Hello from the donation key')
    ).not.toBeInTheDocument()
    view.unmount()
    view.client.clear()
  } finally {
    stream.restore()
  }
})

it('keeps the test response of the previous record away from the next record', async () => {
  const first = pendingReviewItem()
  const second = pendingReviewItem({
    id: '66666666-6666-4666-8666-666666666666',
    line: 2,
  })
  let current: DonationRecord = {
    item: first,
    batch: batchWithItems([first, second]),
    reward: null,
    events: [],
  }
  api.defaults.adapter = async (config) => {
    const url = config.url ?? ''
    if (url.endsWith('/review-context')) {
      return response(config, reviewContextFixture())
    }
    if (url.endsWith('/tests')) {
      return response(
        config,
        successfulTestResult({
          item_id: first.id,
          test_id: String(config.headers.get('Idempotency-Key')),
        })
      )
    }
    if (url === '/api/donations/admin/records') {
      return response(config, {
        page: 1,
        page_size: 20,
        total: 2,
        items: [
          { ...current, item: first },
          { ...current, item: second },
        ],
      })
    }
    if (url.includes('/api/donations/admin/records/')) {
      return response(config, current)
    }
    throw new Error(`Unexpected test endpoint: ${url}`)
  }
  const view = await renderDonation(<DonationRecords />)
  const user = userEvent.setup()
  const detailsButtons = await screen.findAllByRole('button', {
    name: 'Details',
  })
  await user.click(detailsButtons[0] as HTMLButtonElement)
  const dialog = await screen.findByRole('dialog', { name: 'Donation details' })
  const panel = await within(dialog).findByRole('region', {
    name: 'Model test',
  })
  await user.type(within(panel).getByLabelText('User prompt'), 'Say hello')
  await user.click(
    within(panel).getByRole('switch', { name: 'Stream the response' })
  )
  await user.click(within(panel).getByRole('button', { name: 'Run test' }))
  await within(panel).findByText('Hello from the donation key')
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Donation details' })
    ).not.toBeInTheDocument()
  )
  current = {
    item: second,
    batch: batchWithItems([second]),
    reward: null,
    events: [],
  }
  await user.click(detailsButtons[1] as HTMLButtonElement)
  const nextDialog = await screen.findByRole('dialog', {
    name: 'Donation details',
  })
  const nextPanel = await within(nextDialog).findByRole('region', {
    name: 'Model test',
  })
  expect(
    within(nextPanel).queryByText('Hello from the donation key')
  ).not.toBeInTheDocument()
  expect(
    within(nextPanel).queryByText('Test succeeded')
  ).not.toBeInTheDocument()
  view.unmount()
  view.client.clear()
})

it('renders the staging countdown while the interface language is zhCN', async () => {
  // `i18n.language` carries the project's non-standard `zhCN` / `zhTW` codes,
  // which `Intl.RelativeTimeFormat` rejects with "Invalid language tag" unless
  // they are mapped through `toIntlLocale` first. The suite runs in `en`, so
  // only an explicit switch exercises the code path users actually hit.
  await act(async () => {
    await i18n.changeLanguage('zhCN')
  })
  try {
    reviewNetwork({
      record: () =>
        pendingRecord({
          effective_mode: 'manual_review',
          review_action_id: 'action-uuid',
        }),
    })
    await renderDonation(<DonationRecords />)
    expect(
      await screen.findByText(/Temporary storage expires/)
    ).toBeVisible()
  } finally {
    await act(async () => {
      await i18n.changeLanguage('en')
    })
  }
})
