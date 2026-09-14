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
import {
  CanceledError,
  isAxiosError,
  isCancel,
  type AxiosRequestConfig,
} from 'axios'

import { api, getFreshAuthHeaders } from '@/lib/api'
import {
  requireServerSuccess,
  safeServerErrorMessage,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import type {
  CampaignWrite,
  DonationBatchDetail,
  DonationCampaign,
  DonationConnection,
  DonationGroup,
  DonationPage,
  DonationRecord,
  DonationRecordFilters,
  DonationSession,
  DonationSubmission,
  ManagedCampaign,
} from './types'

export function donationSessionIsCurrent(session: DonationSession): boolean {
  const auth = useAuthStore.getState().auth
  return (
    session.userId > 0 &&
    session.sid !== '' &&
    auth.user?.id === session.userId &&
    auth.session?.sid === session.sid
  )
}

export class DonationRequestError extends Error {
  readonly [safeServerErrorMessage] = true
  readonly code: string
  constructor(
    code: string,
    readonly status: number
  ) {
    super(donationErrorKey(code, status))
    this.code = [
      'DONATION_CONFLICT',
      'DONATION_INVALID_REQUEST',
      'DONATION_UNAVAILABLE',
      'DONATION_TARGET_UNAVAILABLE',
      'DONATION_TARGET_CHANGED',
      'DONATION_INSTANCE_MISMATCH',
      'DONATION_IDENTITY_UNAVAILABLE',
      'DONATION_INTEGRATION_UNAUTHORIZED',
      'DONATION_PROTOCOL_MISMATCH',
      'DONATION_NOT_FOUND',
    ].includes(code)
      ? code
      : 'DONATION_ERROR'
    this.name = 'DonationRequestError'
  }
}

function donationErrorKey(code: string, status: number): string {
  if (status === 401) return 'Session expired!'
  if (status === 403) {
    return 'You do not have permission to perform this action.'
  }
  switch (code) {
    case 'DONATION_CONFLICT':
      return 'The original submission has different content. Use the same keys and line positions.'
    case 'DONATION_INVALID_REQUEST':
      return 'The donation settings or input are invalid. Check the fields and try again.'
    case 'DONATION_UNAVAILABLE':
    case 'DONATION_TARGET_UNAVAILABLE':
      return 'This donation target is unavailable. Refresh and try again.'
    case 'DONATION_TARGET_CHANGED':
      return 'The selected group changed. Refresh its configuration before submitting.'
    case 'DONATION_INSTANCE_MISMATCH':
      return 'This connection belongs to a different integration instance.'
    case 'DONATION_IDENTITY_UNAVAILABLE':
      return 'Donation identity material needs administrator attention.'
    case 'DONATION_INTEGRATION_UNAUTHORIZED':
      return 'The integration credential could not be verified.'
    case 'DONATION_PROTOCOL_MISMATCH':
      return 'The donation integration version is not supported.'
    case 'DONATION_NOT_FOUND':
      return 'Donation record not found.'
    default:
      return 'The donation request could not be completed. Try again.'
  }
}

interface Envelope<T> {
  success: boolean
  message: string
  code?: string
  data: T
}

/** Bind both the transport and its result to the originating login session.
 * Never expose Axios config/cause (which can contain key text) to query caches. */
export async function donationRequest<T>(
  session: DonationSession,
  path: string,
  options: AxiosRequestConfig = {},
  signal?: AbortSignal
): Promise<T> {
  const origin = { ...session }
  const controller = new AbortController()
  const abort = () => controller.abort()
  const unsubscribe = useAuthStore.subscribe(() => {
    if (!donationSessionIsCurrent(origin)) abort()
  })
  signal?.addEventListener('abort', abort, { once: true })
  try {
    if (!donationSessionIsCurrent(origin) || signal?.aborted) {
      throw new CanceledError()
    }
    const headers = await getFreshAuthHeaders()
    if (!donationSessionIsCurrent(origin) || controller.signal.aborted) {
      throw new CanceledError()
    }
    const response = await api.request<Envelope<T>>({
      ...options,
      url: `/api/donations${path}`,
      headers: { ...headers, ...options.headers },
      signal: controller.signal,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    })
    if (!donationSessionIsCurrent(origin) || controller.signal.aborted) {
      throw new CanceledError()
    }
    if (response.data?.success !== true) {
      throw new DonationRequestError(response.data?.code ?? '', response.status)
    }
    return requireServerSuccess(response.data).data
  } catch (error) {
    if (
      controller.signal.aborted ||
      !donationSessionIsCurrent(origin) ||
      isCancel(error)
    ) {
      throw new CanceledError()
    }
    if (error instanceof DonationRequestError) throw error
    const payload: unknown = isAxiosError(error)
      ? error.response?.data
      : undefined
    const code =
      payload &&
      typeof payload === 'object' &&
      'code' in payload &&
      typeof payload.code === 'string'
        ? payload.code
        : ''
    throw new DonationRequestError(
      code,
      isAxiosError(error) ? (error.response?.status ?? 0) : 0
    )
  } finally {
    unsubscribe()
    signal?.removeEventListener('abort', abort)
  }
}

export const donationApi = {
  campaigns: (session: DonationSession, signal?: AbortSignal) =>
    donationRequest<DonationCampaign[]>(session, '/campaigns', {}, signal),
  batches: (
    session: DonationSession,
    page: number,
    size: number,
    signal?: AbortSignal
  ) =>
    donationRequest<DonationPage<DonationBatchDetail>>(
      session,
      '/batches',
      { params: { p: page, page_size: size } },
      signal
    ),
  batch: (session: DonationSession, id: string, signal?: AbortSignal) =>
    donationRequest<DonationBatchDetail>(
      session,
      `/batches/${encodeURIComponent(id)}`,
      {},
      signal
    ),
  submit: (
    session: DonationSession,
    input: DonationSubmission,
    requestKey: string,
    signal?: AbortSignal
  ) =>
    donationRequest<DonationBatchDetail>(
      session,
      '/batches',
      {
        method: 'POST',
        data: { campaign_id: input.campaign_id, keys_text: input.keys_text },
        headers: { 'Idempotency-Key': requestKey },
      },
      signal
    ),
  retry: (
    session: DonationSession,
    id: string,
    requestKey: string,
    signal?: AbortSignal
  ) =>
    donationRequest<DonationBatchDetail>(
      session,
      `/batches/${encodeURIComponent(id)}/retry`,
      { method: 'POST', data: {}, headers: { 'Idempotency-Key': requestKey } },
      signal
    ),
  connection: (session: DonationSession, signal?: AbortSignal) =>
    donationRequest<DonationConnection>(
      session,
      '/admin/connection',
      {},
      signal
    ),
  saveConnection: (
    session: DonationSession,
    input: { base_url: string; token?: string },
    signal?: AbortSignal
  ) =>
    donationRequest<DonationConnection>(
      session,
      '/admin/connection',
      { method: 'PUT', data: input },
      signal
    ),
  groups: (session: DonationSession, signal?: AbortSignal) =>
    donationRequest<DonationGroup[]>(
      session,
      '/admin/group-options',
      {},
      signal
    ),
  managedCampaigns: (session: DonationSession, signal?: AbortSignal) =>
    donationRequest<ManagedCampaign[]>(session, '/admin/campaigns', {}, signal),
  saveCampaign: (
    session: DonationSession,
    input: CampaignWrite,
    id?: number,
    signal?: AbortSignal
  ) =>
    donationRequest<ManagedCampaign>(
      session,
      `/admin/campaigns${id ? `/${id}` : ''}`,
      { method: id ? 'PATCH' : 'POST', data: input },
      signal
    ),
  records: (
    session: DonationSession,
    filters: DonationRecordFilters,
    signal?: AbortSignal
  ) =>
    donationRequest<DonationPage<DonationRecord>>(
      session,
      '/admin/records',
      { params: filters },
      signal
    ),
  record: (session: DonationSession, id: string, signal?: AbortSignal) =>
    donationRequest<DonationRecord>(
      session,
      `/admin/records/${encodeURIComponent(id)}`,
      {},
      signal
    ),
}

export function donationBatchIsProcessing(batch: DonationBatchDetail): boolean {
  return (
    batch.reception_state === 'unconfirmed' ||
    batch.summary.processing > 0 ||
    batch.items.some(
      (item) => item.state === 'accepted' && item.reward_state !== 'rewarded'
    )
  )
}

export function donationQueryKey(
  session: DonationSession,
  ...parts: unknown[]
): unknown[] {
  return ['donations', session.key, ...parts]
}
