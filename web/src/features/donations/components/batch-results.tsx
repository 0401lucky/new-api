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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isCancel } from 'axios'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  donationApi,
  donationBatchIsProcessing,
  donationQueryKey,
  donationSessionIsCurrent,
} from '../api'
import { useDonationLifetime } from '../hooks/use-donation-session'
import { reasonLabel } from '../lib/labels'
import type { DonationBatchDetail, DonationSession } from '../types'
import { DonationItemResults } from './item-results'

export function DonationBatchResults(props: {
  session: DonationSession
  batchId: string
  onResume: (batch: DonationBatchDetail) => void
  onObserved?: (batch: DonationBatchDetail) => void
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const client = useQueryClient()
  const actionKey = useRef<string | null>(null)
  const getSignal = useDonationLifetime(props.session, () => {
    actionKey.current = null
  })
  const query = useQuery({
    queryKey: donationQueryKey(props.session, 'batch', props.batchId),
    queryFn: ({ signal }) =>
      donationApi.batch(props.session, props.batchId, signal),
    refetchInterval: (query) =>
      query.state.data && donationBatchIsProcessing(query.state.data)
        ? 5000
        : false,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const retry = useMutation({
    mutationFn: () => {
      actionKey.current ??= crypto.randomUUID()
      return donationApi.retry(
        props.session,
        props.batchId,
        actionKey.current,
        getSignal()
      )
    },
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
    onSuccess: (batch) => {
      if (!donationSessionIsCurrent(props.session)) return
      actionKey.current = null
      client.setQueryData(
        donationQueryKey(props.session, 'batch', props.batchId),
        batch
      )
      void client.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'batches'),
      })
    },
    onError: (error) => {
      if (donationSessionIsCurrent(props.session) && !isCancel(error)) {
        handleServerError(error)
      }
    },
  })
  const onObserved = props.onObserved
  useEffect(() => {
    if (query.data) onObserved?.(query.data)
  }, [query.data, onObserved])
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Unable to load donation results')}
        description={t(query.error.message)}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const batch = query.data
  return (
    <Card role='region' aria-label={t('Submission results')}>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex min-w-0 flex-col gap-1'>
            <CardTitle>{t('Submission results')}</CardTitle>
            <p className='text-muted-foreground text-sm break-words'>
              {batch.campaign_name} ·{' '}
              {formatTimestampToDate(batch.created_at_ms, 'milliseconds')}
            </p>
          </div>
          <Button
            variant='outline'
            size='sm'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Refresh')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        <dl
          aria-live='polite'
          className='grid grid-cols-2 gap-4 sm:grid-cols-4'
        >
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Accepted keys')}
            </dt>
            <dd className='text-xl font-semibold tabular-nums'>
              {batch.summary.accepted} / {batch.summary.total}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Invalid / duplicate')}
            </dt>
            <dd className='text-xl font-semibold tabular-nums'>
              {batch.summary.invalid} / {batch.summary.duplicate}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Processing')}</dt>
            <dd className='text-xl font-semibold tabular-nums'>
              {batch.summary.processing}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Permanent quota credited')}
            </dt>
            <dd className='text-xl font-semibold tabular-nums'>
              {formatQuota(batch.summary.rewarded_quota)}
            </dd>
          </div>
        </dl>
        {batch.reception_state === 'unconfirmed' && (
          <Alert>
            <AlertDescription className='flex flex-wrap items-center justify-between gap-3'>
              <span>
                {t(
                  'Receipt is not confirmed. Keep the original keys and retry this submission.'
                )}
              </span>
              <Button variant='outline' onClick={() => props.onResume(batch)}>
                {t('Resume submission')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {(batch.summary.pending_review ?? 0) > 0 && (
          <Alert>
            <AlertDescription>
              {t(
                '{{count}} keys are waiting for manual review. Rewards are credited only after an administrator approves the keys and they are received.',
                { count: batch.summary.pending_review }
              )}
            </AlertDescription>
          </Alert>
        )}
        {batch.last_error && (
          <p className='text-muted-foreground text-sm'>
            {reasonLabel(batch.last_error, t)}
          </p>
        )}
        <DonationItemResults items={batch.items} />
        {batch.reception_state === 'confirmed' &&
          batch.items.some(
            (item) => item.retryable && item.state === 'retry_pending'
          ) && (
            <div className='flex flex-wrap items-center gap-3'>
              <Button
                variant='outline'
                disabled={retry.isPending}
                onClick={() => retry.mutate()}
              >
                {retry.isPending && <Spinner data-icon='inline-start' />}
                {t('Retry pending keys')}
              </Button>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Accepted keys are not submitted again. Rewards are credited only once.'
                )}
              </p>
            </div>
          )}
      </CardContent>
    </Card>
  )
}
