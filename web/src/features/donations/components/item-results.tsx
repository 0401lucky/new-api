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
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatQuota,
  formatTimestampRelative,
  formatTimestampToDate,
} from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  intakeLabel,
  intakeVariant,
  reasonLabel,
  rewardLabel,
} from '../lib/labels'
import type { DonationItem } from '../types'

export function DonationRewardStatus(props: { item: DonationItem }) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  return (
    <div className='flex flex-col items-start gap-1'>
      <StatusBadge
        label={rewardLabel(props.item.reward_state, t)}
        variant={props.item.reward_state === 'rewarded' ? 'success' : 'neutral'}
        copyable={false}
      />
      {props.item.reward_state === 'rewarded' && (
        <span className='text-sm tabular-nums'>
          {t('Permanent quota: {{quota}}', {
            quota: formatQuota(props.item.rewarded_quota),
          })}
        </span>
      )}
      {props.item.reward_reason && (
        <span className='text-muted-foreground text-xs'>
          {reasonLabel(props.item.reward_reason, t)}
        </span>
      )}
    </div>
  )
}

export function DonationIntakeStatus(props: { item: DonationItem }) {
  const { t, i18n } = useTranslation()
  const awaitingReview = props.item.state === 'pending_review'
  const expired = props.item.reason_code === 'staging_expired'
  return (
    <div className='flex flex-col items-start gap-1'>
      <StatusBadge
        label={
          expired
            ? t('Temporary storage expired')
            : intakeLabel(props.item.state, t)
        }
        variant={expired ? 'warning' : intakeVariant(props.item.state)}
        copyable={false}
      />
      {props.item.reason_code &&
        props.item.reason_code !== 'review_rejected' && (
          <span className='text-muted-foreground text-xs'>
            {reasonLabel(props.item.reason_code, t)}
          </span>
        )}
      {props.item.review_note && (
        <span className='text-muted-foreground text-xs'>
          {props.item.review_note}
        </span>
      )}
      {awaitingReview && props.item.staging_expires_at_ms ? (
        <span className='text-muted-foreground text-xs'>
          {t('Temporary storage expires')}{' '}
          {formatTimestampToDate(
            props.item.staging_expires_at_ms,
            'milliseconds'
          )}{' '}
          (
          {formatTimestampRelative(
            props.item.staging_expires_at_ms,
            'milliseconds',
            toIntlLocale(i18n.language)
          )}
          )
        </span>
      ) : null}
    </div>
  )
}

export function DonationItemResults(props: { items: DonationItem[] }) {
  const { t } = useTranslation()
  const columns = useMemo<ColumnDef<DonationItem>[]>(
    () => [
      {
        accessorKey: 'line',
        header: t('Original line'),
        cell: ({ row }) => row.original.line,
        meta: { label: t('Original line') },
      },
      {
        accessorKey: 'key_mask',
        header: t('Key'),
        cell: ({ row }) => (
          <span className='font-mono'>{row.original.key_mask}</span>
        ),
        meta: { label: t('Key') },
      },
      {
        id: 'intake',
        header: t('Key result'),
        cell: ({ row }) => <DonationIntakeStatus item={row.original} />,
        meta: { label: t('Key result') },
      },
      {
        id: 'reward',
        header: t('Reward'),
        cell: ({ row }) => <DonationRewardStatus item={row.original} />,
        meta: { label: t('Reward') },
      },
    ],
    [t]
  )
  const { table } = useDataTable({
    columns,
    data: props.items,
    getRowId: (item) => item.id,
    enableRowSelection: false,
    enableSorting: false,
    withPaginationRowModel: false,
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
  })
  return (
    <DataTablePage
      table={table}
      columns={columns}
      toolbarProps={null}
      showPagination={false}
      paginationInFooter={false}
      className='h-auto'
    />
  )
}
