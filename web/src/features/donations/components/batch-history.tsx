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
import { useQuery } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  donationApi,
  donationBatchIsProcessing,
  donationQueryKey,
} from '../api'
import type { DonationBatchDetail, DonationSession } from '../types'

const EMPTY_BATCHES: DonationBatchDetail[] = []

export function DonationBatchHistory(props: {
  session: DonationSession
  onOpen: (id: string) => void
}) {
  const { t } = useTranslation()
  const onOpen = props.onOpen
  useSystemConfigStore((state) => state.config.currency)
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const query = useQuery({
    queryKey: donationQueryKey(props.session, 'batches', pagination),
    queryFn: ({ signal }) =>
      donationApi.batches(
        props.session,
        pagination.pageIndex + 1,
        pagination.pageSize,
        signal
      ),
    refetchInterval: (query) =>
      query.state.data?.items.some(donationBatchIsProcessing) ? 5000 : false,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const columns = useMemo<ColumnDef<DonationBatchDetail>[]>(
    () => [
      {
        accessorKey: 'created_at_ms',
        header: t('Submitted at'),
        cell: ({ row }) =>
          formatTimestampToDate(row.original.created_at_ms, 'milliseconds'),
        meta: { label: t('Submitted at') },
      },
      {
        accessorKey: 'campaign_name',
        header: t('Campaign'),
        meta: { label: t('Campaign') },
      },
      {
        id: 'result',
        header: t('Accepted keys'),
        cell: ({ row }) =>
          `${row.original.summary.accepted} / ${row.original.summary.total}`,
        meta: { label: t('Accepted keys') },
      },
      {
        id: 'reward',
        header: t('Permanent quota credited'),
        cell: ({ row }) => formatQuota(row.original.summary.rewarded_quota),
        meta: { label: t('Permanent quota credited') },
      },
      {
        id: 'details',
        header: t('Details'),
        cell: ({ row }) => (
          <Button
            variant='ghost'
            size='sm'
            onClick={() => onOpen(row.original.id)}
          >
            {t('View results')}
          </Button>
        ),
        meta: { label: t('Details') },
      },
    ],
    [t, onOpen]
  )
  const { table } = useDataTable({
    columns,
    data: query.isError ? EMPTY_BATCHES : (query.data?.items ?? EMPTY_BATCHES),
    getRowId: (batch) => batch.id,
    totalCount: query.data?.total ?? 0,
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    manualFiltering: true,
    enableRowSelection: false,
    enableSorting: false,
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
  })
  return (
    <section aria-label={t('Donation history')} className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-base font-semibold'>{t('Donation history')}</h3>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          {t('Refresh')}
        </Button>
      </div>
      {query.isError ? (
        <ErrorState
          title={t('Unable to load donation history')}
          description={t(query.error.message)}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={query.isPending}
          isFetching={query.isFetching}
          emptyTitle={t('No donations yet')}
          emptyDescription={t(
            'Your submissions and permanent rewards will appear here.'
          )}
          toolbarProps={null}
          paginationInFooter={false}
          className='h-auto'
        />
      )}
    </section>
  )
}
