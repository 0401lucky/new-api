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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CanceledError, isCancel } from 'axios'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'

import { handleServerError } from '@/lib/handle-server-error'

import {
  donationApi,
  DonationRequestError,
  donationQueryKey,
  donationSessionIsCurrent,
} from '../api'
import {
  donationSubmissionSchema,
  type DonationSubmissionValues,
} from '../lib/schema'
import type { DonationBatchDetail, DonationSession } from '../types'
import { useDonationLifetime } from './use-donation-session'

interface SubmissionIdentity {
  requestKey: string
  generation: number
}
interface SubmissionRequest
  extends DonationSubmissionValues, SubmissionIdentity {}

export function useDonationSubmission(props: {
  session: DonationSession
  resume: DonationBatchDetail | null
  observedBatch?: DonationBatchDetail | null
  onResult: (batch: DonationBatchDetail) => void
}) {
  const queryClient = useQueryClient()
  const sequence = useRef(0)
  const active = useRef<(SubmissionIdentity & { confirmed: boolean }) | null>(
    null
  )
  const request = useRef<SubmissionRequest | null>(null)
  const inFlight = useRef<{
    input: SubmissionRequest
    controller: AbortController
  } | null>(null)
  const [unconfirmed, setUnconfirmed] = useState(false)
  const form = useForm<DonationSubmissionValues>({
    resolver: zodResolver(donationSubmissionSchema),
    defaultValues: {
      campaign_id: props.resume?.campaign_id ?? 0,
      keys_text: '',
    },
  })
  useDonationLifetime(props.session, () => {
    inFlight.current?.controller.abort()
    inFlight.current = null
    request.current = null
    active.current = null
    form.setValue('keys_text', '')
  })
  const resumeKey = props.resume?.request_key
  const resumeCampaign = props.resume?.campaign_id
  const observedBatch = props.observedBatch

  useEffect(() => {
    if (
      !resumeKey ||
      !resumeCampaign ||
      active.current?.requestKey === resumeKey
    ) {
      return
    }
    inFlight.current?.controller.abort()
    inFlight.current = null
    request.current = null
    active.current = {
      requestKey: resumeKey,
      generation: ++sequence.current,
      confirmed: false,
    }
    form.reset({ campaign_id: resumeCampaign, keys_text: '' })
    setUnconfirmed(false)
  }, [form, resumeKey, resumeCampaign])

  useEffect(() => {
    if (
      !active.current ||
      active.current.confirmed ||
      observedBatch?.request_key !== active.current.requestKey ||
      observedBatch.reception_state === 'unconfirmed'
    ) {
      return
    }
    // Confirmation is monotonic even if an older POST finishes afterwards.
    active.current.confirmed = true
    if (inFlight.current?.input.requestKey === active.current.requestKey) {
      inFlight.current.controller.abort()
    }
    request.current = null
    form.setValue('keys_text', '')
    form.clearErrors()
    setUnconfirmed(false)
  }, [form, observedBatch])

  const isCurrentAttempt = (
    identity: SubmissionIdentity | null | undefined
  ): boolean =>
    Boolean(
      identity &&
      active.current &&
      identity.requestKey === active.current.requestKey &&
      identity.generation === active.current.generation &&
      !active.current.confirmed &&
      donationSessionIsCurrent(props.session)
    )

  const mutation = useMutation({
    // Variables contain only non-secret identity. The body stays in a separate
    // snapshot, and an obsolete mutation can never consume a newer draft.
    mutationFn: async (identity: SubmissionIdentity) => {
      const operation = inFlight.current
      if (
        !operation ||
        operation.input.generation !== identity.generation ||
        operation.input.requestKey !== identity.requestKey
      ) {
        throw new CanceledError()
      }
      return donationApi.submit(
        props.session,
        operation.input,
        operation.input.requestKey,
        operation.controller.signal
      )
    },
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
    onSuccess: (batch, identity) => {
      if (!isCurrentAttempt(identity)) return
      queryClient.setQueryData(
        donationQueryKey(props.session, 'batch', batch.id),
        batch
      )
      void queryClient.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'batches'),
      })
      const confirmed = batch.reception_state !== 'unconfirmed'
      if (active.current) active.current.confirmed = confirmed
      setUnconfirmed(!confirmed)
      if (confirmed) {
        request.current = null
        form.setValue('keys_text', '')
        form.clearErrors()
      }
      props.onResult(batch)
    },
    onError: (error, identity) => {
      if (!isCurrentAttempt(identity) || isCancel(error)) return
      const uncertain =
        !(error instanceof DonationRequestError) ||
        error.status === 0 ||
        error.status >= 500
      setUnconfirmed(uncertain)
      if (!uncertain) request.current = null
      form.setError('root', { message: error.message })
      handleServerError(error)
    },
    onSettled: (_data, _error, identity) => {
      if (
        identity &&
        inFlight.current?.input.generation === identity.generation
      ) {
        inFlight.current = null
      }
    },
  })

  const submit = form.handleSubmit((values) => {
    if (inFlight.current) return
    form.clearErrors('root')
    if (!request.current) {
      const requestKey = resumeKey ?? crypto.randomUUID()
      if (active.current?.requestKey !== requestKey) {
        active.current = {
          requestKey,
          generation: ++sequence.current,
          confirmed: false,
        }
      }
      request.current = {
        ...values,
        requestKey,
        generation: active.current.generation,
      }
    }
    inFlight.current = {
      input: request.current,
      controller: new AbortController(),
    }
    mutation.mutate({
      requestKey: request.current.requestKey,
      generation: request.current.generation,
    })
  })

  const reset = () => {
    inFlight.current?.controller.abort()
    inFlight.current = null
    request.current = null
    active.current = null
    setUnconfirmed(false)
    form.reset({ campaign_id: 0, keys_text: '' })
  }
  return { form, mutation, submit, reset, unconfirmed }
}
