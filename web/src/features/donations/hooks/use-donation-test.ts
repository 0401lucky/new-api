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
import { isCancel } from 'axios'
import { useCallback, useRef, useState } from 'react'
import { SSE } from 'sse.js'

import { getFreshAuthHeaders } from '@/lib/api'

import {
  donationApi,
  donationSessionIsCurrent,
  DonationRequestError,
} from '../api'
import {
  donationTestDoneSchema,
  donationTestMetaSchema,
  donationTestResultSchema,
} from '../lib/schema'
import type {
  DonationReviewContext,
  DonationSession,
  DonationTestIntent,
  DonationTestMetadata,
  DonationTestResult,
} from '../types'
import { useDonationLifetime } from './use-donation-session'

const TEST_ENDPOINT = '/api/donations/admin/records'
const STREAM_CLOSED_READY_STATE = 2

interface DonationStreamEvent extends Event {
  data?: string
  readyState?: number
  responseCode?: number
  headers?: Record<string, string[]>
}

export interface DonationTestRun {
  running: boolean
  text: string
  result: DonationTestResult | null
  error: string
}

const IDLE_RUN: DonationTestRun = {
  running: false,
  text: '',
  result: null,
  error: '',
}

function parseEventData(data: string | undefined): unknown {
  if (!data) return null
  try {
    return JSON.parse(data)
  } catch {
    return null
  }
}

/** Runs a single real call for one donation record. The receiver re-frames the
 * upstream protocol as `meta` / `delta` / `done`, so this hook listens to those
 * named events instead of the Playground's OpenAI chunk parser.
 *
 * Only the record that is currently open may be tested: unmounting the panel,
 * switching records or changing the signed-in session closes the stream and
 * discards any late response. */
export function useDonationTest(
  session: DonationSession,
  context: DonationReviewContext
) {
  const [run, setRun] = useState<DonationTestRun>(IDLE_RUN)
  const sourceRef = useRef<{ close: () => void } | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const generationRef = useRef(0)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cancelActive = useCallback(() => {
    generationRef.current += 1
    sourceRef.current?.close()
    sourceRef.current = null
    abortRef.current?.abort()
    abortRef.current = null
    if (timeoutRef.current !== null) clearTimeout(timeoutRef.current)
    timeoutRef.current = null
  }, [])
  // Aborting here covers closing the detail, switching records, switching
  // account and signing out, all of which must drop the running test.
  useDonationLifetime(session, () => {
    cancelActive()
    setRun(IDLE_RUN)
  })

  const start = useCallback(
    (itemId: string, intent: DonationTestIntent) => {
      cancelActive()
      if (!donationSessionIsCurrent(session)) return
      const generation = generationRef.current
      const testId = crypto.randomUUID()
      setRun({ running: true, text: '', result: null, error: '' })
      const isCurrent = () =>
        generationRef.current === generation &&
        donationSessionIsCurrent(session)
      let text = ''
      let settled = false
      const limits = context.review_limits
      const byteLength = (value: string) =>
        new TextEncoder().encode(value).length
      const finish = (result: DonationTestResult | null, error: string) => {
        if (!isCurrent() || settled) return
        settled = true
        if (timeoutRef.current !== null) clearTimeout(timeoutRef.current)
        timeoutRef.current = null
        const source = sourceRef.current
        sourceRef.current = null
        source?.close()
        abortRef.current?.abort()
        abortRef.current = null
        setRun({ running: false, text, result, error })
      }
      const matchesRequest = (
        result: Pick<
          DonationTestMetadata,
          | 'test_id'
          | 'item_id'
          | 'batch_id'
          | 'model'
          | 'start_revision'
          | 'target_revision'
        >
      ) =>
        result.test_id === testId &&
        result.item_id === itemId &&
        result.batch_id === context.batch_id &&
        result.model === intent.model &&
        result.start_revision === intent.expected_item_revision &&
        result.target_revision === intent.review_target_revision
      const acceptResult = (value: unknown, metadataOnly: boolean) => {
        const parsed = donationTestResultSchema.safeParse(value)
        if (
          !parsed.success ||
          !matchesRequest(parsed.data) ||
          parsed.data.stream !== intent.stream ||
          byteLength(parsed.data.text ?? '') > limits.max_response_bytes ||
          (!metadataOnly &&
            parsed.data.state === 'succeeded' &&
            !parsed.data.text?.trim())
        ) {
          finish(null, 'The test result could not be read.')
          return
        }
        text = metadataOnly ? '' : (parsed.data.text ?? '')
        const result = { ...parsed.data }
        if (metadataOnly) delete result.text
        finish(
          result,
          metadataOnly || !text
            ? 'The saved test result is available, but its response text is not retained.'
            : ''
        )
      }
      if (byteLength(JSON.stringify(intent)) > limits.max_request_bytes) {
        finish(null, 'Please shorten the prompt or the system prompt.')
        return
      }
      timeoutRef.current = setTimeout(
        () => {
          finish(null, 'The model test timed out before it reported a result.')
        },
        Math.min(
          limits.total_timeout_seconds * 1000,
          Math.max(1, context.expires_at_ms - Date.now())
        )
      )

      if (!intent.stream) {
        const controller = new AbortController()
        abortRef.current = controller
        void (async () => {
          try {
            const result = await donationApi.runTest(
              session,
              itemId,
              testId,
              intent,
              controller.signal
            )
            if (!isCurrent()) return
            acceptResult(result, !('text' in result))
          } catch (error) {
            if (!isCurrent() || isCancel(error)) return
            let reason = error instanceof Error ? error.message : 'Test failed'
            if (error instanceof DonationRequestError && error.status === 409) {
              reason = 'This record cannot be tested right now.'
            }
            finish(null, reason)
          }
        })()
        return
      }

      let meta: ReturnType<typeof donationTestMetaSchema.parse> | null = null
      let metadataOnly = false
      void (async () => {
        let headers: Record<string, string>
        try {
          headers = await getFreshAuthHeaders()
        } catch {
          finish(null, 'The test could not be started.')
          return
        }
        if (!isCurrent() || settled) return
        const source = new SSE(
          `${TEST_ENDPOINT}/${encodeURIComponent(itemId)}/tests`,
          {
            method: 'POST',
            payload: JSON.stringify(intent),
            headers: {
              ...headers,
              'Content-Type': 'application/json',
              Accept: 'text/event-stream',
              'Idempotency-Key': testId,
            },
            withCredentials: true,
            autoReconnect: false,
            start: false,
          }
        )
        sourceRef.current = source
        source.addEventListener('open', (event: DonationStreamEvent) => {
          metadataOnly = Boolean(
            event.headers?.['content-type']?.some((value) =>
              value.startsWith('application/json')
            )
          )
        })
        source.addEventListener('meta', (event: DonationStreamEvent) => {
          if (!isCurrent() || settled) return
          const parsed = donationTestMetaSchema.safeParse(
            parseEventData(event.data)
          )
          if (
            meta ||
            byteLength(event.data ?? '') > limits.max_event_bytes ||
            !parsed.success ||
            !matchesRequest(parsed.data)
          ) {
            finish(null, 'The test result could not be read.')
            return
          }
          meta = parsed.data
        })
        source.addEventListener('delta', (event: DonationStreamEvent) => {
          if (!isCurrent() || settled) return
          const parsed = parseEventData(event.data)
          if (
            !meta ||
            byteLength(event.data ?? '') > limits.max_event_bytes ||
            !parsed ||
            typeof parsed !== 'object' ||
            !('text' in parsed) ||
            typeof parsed.text !== 'string'
          ) {
            finish(null, 'The test result could not be read.')
            return
          }
          if (
            byteLength(text) + byteLength(parsed.text) >
            limits.max_response_bytes
          ) {
            finish(null, 'The test response exceeded the supported size limit.')
            return
          }
          text += parsed.text
          setRun((previous) => ({ ...previous, text }))
        })
        source.addEventListener('done', (event: DonationStreamEvent) => {
          if (!isCurrent() || settled) return
          const parsed = donationTestDoneSchema.safeParse(
            parseEventData(event.data)
          )
          if (
            !meta ||
            byteLength(event.data ?? '') > limits.max_event_bytes ||
            !parsed.success ||
            (parsed.data.state === 'succeeded' && !text.trim())
          ) {
            finish(null, 'The test result could not be read.')
            return
          }
          finish({ ...meta, ...parsed.data, stream: true, text }, '')
        })
        source.addEventListener('error', (event: DonationStreamEvent) => {
          const body = parseEventData(event.data)
          const code =
            body &&
            typeof body === 'object' &&
            'code' in body &&
            typeof body.code === 'string'
              ? body.code
              : ''
          finish(
            null,
            event.responseCode && event.responseCode >= 400
              ? new DonationRequestError(code, event.responseCode).message
              : 'The model test stream failed.'
          )
        })
        source.addEventListener(
          'readystatechange',
          (event: DonationStreamEvent) => {
            if (!isCurrent() || settled) return
            if (event.readyState === STREAM_CLOSED_READY_STATE) {
              if (metadataOnly && !meta && !text) {
                // Replayed stream requests return saved JSON metadata. Read it
                // through the session-bound client; never re-send the POST.
                const controller = new AbortController()
                abortRef.current = controller
                void donationApi
                  .testMetadata(session, itemId, testId, controller.signal)
                  .then(
                    (result) => acceptResult(result, true),
                    () => finish(null, 'The test result could not be read.')
                  )
                return
              }
              finish(
                null,
                'The model test stream ended before it reported a result.'
              )
            }
          }
        )
        try {
          source.stream()
        } catch {
          finish(null, 'The test could not be started.')
        }
      })()
    },
    [cancelActive, session, context]
  )

  const stop = useCallback(() => {
    cancelActive()
    setRun((previous) =>
      previous.running
        ? { ...previous, running: false, result: null, error: 'Test cancelled' }
        : previous
    )
  }, [cancelActive])

  const reset = useCallback(() => {
    cancelActive()
    setRun(IDLE_RUN)
  }, [cancelActive])

  return { run, start, stop, reset }
}
