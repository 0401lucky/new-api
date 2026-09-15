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
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'

import { reviewReasonLabel } from '../lib/labels'
import type {
  DonationRecord,
  DonationReviewAction,
  DonationTestMetadata,
} from '../types'
import { DonationTestSummary } from './record-test'

export function DonationRecordHistory(props: { record: DonationRecord }) {
  const { t } = useTranslation()
  const reviewColumns: StaticDataTableColumn<DonationReviewAction>[] = [
    {
      id: 'time',
      header: t('Time'),
      cell: (action) =>
        formatTimestampToDate(
          action.applied_at_ms ?? action.created_at_ms,
          'milliseconds'
        ),
    },
    {
      id: 'actor',
      header: t('Operator'),
      cell: (action) => t('Administrator #{{id}}', { id: action.actor_id }),
    },
    {
      id: 'action',
      header: t('Action'),
      cell: (action) => {
        if (action.kind === 'enter_review') return t('Move to manual review')
        return action.kind === 'approve' ? t('Approve') : t('Reject')
      },
    },
    {
      id: 'status',
      header: t('Status'),
      cellClassName: 'whitespace-normal',
      cell: (action) => {
        let label = t('Awaiting confirmation')
        let variant: StatusVariant = 'warning'
        if (action.status === 'applied') {
          label = t('Action applied')
          variant = 'success'
        } else if (action.status === 'rejected') {
          label = t('Action not applied')
          variant = 'neutral'
        }
        return (
          <div className='flex flex-col gap-1'>
            <StatusBadge label={label} variant={variant} copyable={false} />
            {action.reason_code && (
              <span className='text-muted-foreground text-xs'>
                {reviewReasonLabel(action.reason_code, t)}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: 'note',
      header: t('Review note'),
      cellClassName: 'whitespace-normal break-words',
      cell: (action) => action.note || '—',
    },
  ]
  const testColumns: StaticDataTableColumn<DonationTestMetadata>[] = [
    {
      id: 'result',
      header: t('Model test'),
      cellClassName: 'whitespace-normal break-words',
      cell: (test) => <DonationTestSummary test={test} />,
    },
    {
      id: 'actor',
      header: t('Operator'),
      cell: (test) =>
        test.actor_id ? t('Administrator #{{id}}', { id: test.actor_id }) : '—',
    },
  ]
  return (
    <div className='flex flex-col gap-5'>
      <section
        aria-label={t('Recent review actions')}
        className='flex flex-col gap-2'
      >
        <h3 className='font-medium'>{t('Recent review actions')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t('Showing up to 20 recent entries, newest first.')}
        </p>
        <StaticDataTable
          columns={reviewColumns}
          data={props.record.recent_review_actions ?? []}
          getRowKey={(action) => action.action_id}
          emptyContent={t('No review actions yet.')}
          tableProps={{ 'aria-label': t('Recent review actions') }}
        />
      </section>
      <section
        aria-label={t('Recent model tests')}
        className='flex flex-col gap-2'
      >
        <h3 className='font-medium'>{t('Recent model tests')}</h3>
        <p className='text-muted-foreground text-xs'>
          {t('Showing up to 20 recent entries, newest first.')}
        </p>
        <StaticDataTable
          columns={testColumns}
          data={props.record.recent_tests ?? []}
          getRowKey={(test) => test.test_id}
          emptyContent={t('No model tests yet.')}
          tableProps={{ 'aria-label': t('Recent model tests') }}
        />
      </section>
    </div>
  )
}
