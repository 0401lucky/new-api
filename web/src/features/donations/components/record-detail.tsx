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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  donationApi,
  donationQueryKey,
  donationRecordIsProcessing,
} from '../api'
import { donationPermissions } from '../lib/access'
import { intakeLabel, intakeVariant, reasonLabel } from '../lib/labels'
import type { DonationRecord, DonationSession } from '../types'
import { DonationIntakeStatus, DonationRewardStatus } from './item-results'
import { DonationRecordHistory } from './record-history'
import { DonationRecordReview } from './record-review'

type Event = NonNullable<DonationRecord['events']>[number]

export function DonationRecordDetail(props: {
  session: DonationSession
  itemId: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const user = useAuthStore((state) => state.auth.user)
  const permissions = donationPermissions(user)
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: donationQueryKey(props.session, 'record', props.itemId),
    queryFn: ({ signal }) =>
      donationApi.record(props.session, props.itemId, signal),
    refetchInterval: (query) =>
      query.state.data && donationRecordIsProcessing(query.state.data)
        ? 5000
        : false,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const onChanged = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: donationQueryKey(props.session, 'record', props.itemId),
    })
    void queryClient.invalidateQueries({
      queryKey: donationQueryKey(props.session, 'records'),
    })
    void queryClient.invalidateQueries({
      queryKey: donationQueryKey(props.session, 'batches'),
    })
  }, [queryClient, props.session, props.itemId])
  const columns: StaticDataTableColumn<Event>[] = [
    {
      id: 'time',
      header: t('Time'),
      cell: (event) =>
        formatTimestampToDate(event.created_at_ms, 'milliseconds'),
    },
    {
      id: 'state',
      header: t('Status'),
      cell: (event) => (
        <StatusBadge
          label={intakeLabel(event.state, t)}
          variant={intakeVariant(event.state)}
          copyable={false}
        />
      ),
    },
    {
      id: 'reason',
      header: t('Reason'),
      cell: (event) => reasonLabel(event.reason_code, t),
    },
  ]
  const record = query.data
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Donation details')}
      description={t(
        'Historical donation and reward records remain available after a key or group is removed.'
      )}
      contentClassName='sm:max-w-3xl'
    >
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          description={t(query.error.message)}
          onRetry={() => void query.refetch()}
        />
      )}
      {record && (
        <div className='flex flex-col gap-5'>
          <dl className='grid grid-cols-1 gap-4 text-sm sm:grid-cols-2'>
            <div>
              <dt className='text-muted-foreground'>{t('User')}</dt>
              <dd>
                {record.batch.username} · {record.batch.user_id}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Campaign')}</dt>
              <dd>{record.batch.campaign_name}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Receiving group')}</dt>
              <dd>
                {record.batch.group_name} · {record.batch.group_id}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Rule version')}</dt>
              <dd>{record.batch.campaign_version}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Key')}</dt>
              <dd className='font-mono'>{record.item.key_mask}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Original line')}</dt>
              <dd>{record.item.line}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Key result')}</dt>
              <dd className='pt-1'>
                <DonationIntakeStatus item={record.item} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Reward')}</dt>
              <dd className='pt-1'>
                <DonationRewardStatus item={record.item} />
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Credential ID')}</dt>
              <dd>{record.item.credential_id ?? '—'}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Submitted at')}</dt>
              <dd>
                {formatTimestampToDate(
                  record.batch.created_at_ms,
                  'milliseconds'
                )}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Accepted at')}</dt>
              <dd>
                {formatTimestampToDate(
                  record.item.accepted_at_ms ?? undefined,
                  'milliseconds'
                )}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>
                {t('Reward credited at')}
              </dt>
              <dd>
                {formatTimestampToDate(
                  record.reward?.credited_at_ms,
                  'milliseconds'
                )}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>
                {t('Permanent reward per key')}
              </dt>
              <dd>{formatQuota(record.batch.reward_quota)}</dd>
            </div>
            <div className='min-w-0'>
              <dt className='text-muted-foreground'>{t('Record ID')}</dt>
              <dd className='flex items-center gap-1'>
                <span className='min-w-0 font-mono text-xs break-all'>
                  {record.item.id}
                </span>
                <CopyButton value={record.item.id} />
              </dd>
            </div>
            {record.reward && (
              <div className='min-w-0 sm:col-span-2'>
                <dt className='text-muted-foreground'>{t('Reward record')}</dt>
                <dd className='flex items-center gap-1'>
                  <span className='min-w-0 font-mono text-xs break-all'>
                    {record.reward.id}
                  </span>
                  <CopyButton value={record.reward.id} />
                </dd>
              </div>
            )}
          </dl>
          <DonationRecordReview
            session={props.session}
            record={record}
            canReview={permissions.recordsReview}
            canTest={permissions.recordsTest}
            onChanged={onChanged}
          />
          <DonationRecordHistory record={record} />
          <section
            aria-label={t('Processing history')}
            className='flex flex-col gap-2'
          >
            <h3 className='font-medium'>{t('Processing history')}</h3>
            <StaticDataTable
              columns={columns}
              data={record.events ?? []}
              getRowKey={(event) => event.id}
              emptyContent={t('No additional processing events.')}
            />
          </section>
        </div>
      )}
    </Dialog>
  )
}
