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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { donationApi, donationQueryKey } from './api'
import { DonationLayout } from './components/donation-layout'
import {
  DonationIntakeStatus,
  DonationRewardStatus,
} from './components/item-results'
import { DonationRecordDetail } from './components/record-detail'
import { DonationRecordFilterBar } from './components/record-filters'
import { useDonationSession } from './hooks/use-donation-session'
import { donationPermissions } from './lib/access'
import type {
  DonationRecord,
  DonationRecordFilters,
  DonationSession,
} from './types'

const EMPTY_RECORDS: DonationRecord[] = []

export function DonationRecords() {
  const { t } = useTranslation()
  const session = useDonationSession()
  const user = useAuthStore((state) => state.auth.user)
  return (
    <DonationLayout page='records'>
      {donationPermissions(user).recordsRead && session.sid ? (
        <RecordsWorkspace key={session.key} session={session} />
      ) : (
        <ErrorState
          title={t('Access Forbidden')}
          description={t('You do not have permission to perform this action.')}
        />
      )}
    </DonationLayout>
  )
}

function RecordsWorkspace(props: { session: DonationSession }) {
  const { t } = useTranslation()
  const [filters, setFilters] = useState<DonationRecordFilters>({
    p: 1,
    page_size: 20,
  })
  const [detailId, setDetailId] = useState<string | null>(null)
  const query = useQuery({
    queryKey: donationQueryKey(props.session, 'records', filters),
    queryFn: ({ signal }) =>
      donationApi.records(props.session, filters, signal),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const columns = useMemo<ColumnDef<DonationRecord>[]>(
    () => [
      {
        id: 'created_at',
        header: t('Submitted at'),
        cell: ({ row }) =>
          formatTimestampToDate(
            row.original.batch.created_at_ms,
            'milliseconds'
          ),
        meta: { label: t('Submitted at') },
      },
      {
        id: 'user',
        header: t('User'),
        cell: ({ row }) =>
          `${row.original.batch.username} · ${row.original.batch.user_id}`,
        meta: { label: t('User') },
      },
      {
        id: 'campaign',
        header: t('Campaign'),
        cell: ({ row }) => row.original.batch.campaign_name,
        meta: { label: t('Campaign') },
      },
      {
        id: 'key',
        header: t('Key'),
        cell: ({ row }) => (
          <div className='flex flex-col gap-1'>
            <span className='font-mono'>{row.original.item.key_mask}</span>
            <span className='text-muted-foreground text-xs'>
              {t('Line {{line}}', { line: row.original.item.line })}
            </span>
          </div>
        ),
        meta: { label: t('Key') },
      },
      {
        id: 'group',
        header: t('Receiving group'),
        cell: ({ row }) => row.original.batch.group_name,
        meta: { label: t('Receiving group') },
      },
      {
        id: 'credential',
        header: t('Credential ID'),
        cell: ({ row }) => row.original.item.credential_id ?? '—',
        meta: { label: t('Credential ID') },
      },
      {
        id: 'intake',
        header: t('Key result'),
        cell: ({ row }) => <DonationIntakeStatus item={row.original.item} />,
        meta: { label: t('Key result') },
      },
      {
        id: 'reward',
        header: t('Reward'),
        cell: ({ row }) => <DonationRewardStatus item={row.original.item} />,
        meta: { label: t('Reward') },
      },
      {
        id: 'details',
        header: t('Details'),
        cell: ({ row }) => (
          <Button
            variant='ghost'
            size='sm'
            onClick={() => setDetailId(row.original.item.id)}
          >
            {t('Details')}
          </Button>
        ),
        meta: { label: t('Details') },
      },
    ],
    [t]
  )
  const { table } = useDataTable({
    data: query.isError ? EMPTY_RECORDS : (query.data?.items ?? EMPTY_RECORDS),
    columns,
    getRowId: (record) => record.item.id,
    totalCount: query.data?.total ?? 0,
    pagination: { pageIndex: filters.p - 1, pageSize: filters.page_size },
    onPaginationChange: (updater) =>
      setFilters((previous) => {
        const current = {
          pageIndex: previous.p - 1,
          pageSize: previous.page_size,
        }
        const next = typeof updater === 'function' ? updater(current) : updater
        return {
          ...previous,
          p: next.pageSize === previous.page_size ? next.pageIndex + 1 : 1,
          page_size: next.pageSize,
        }
      }),
    manualPagination: true,
    manualFiltering: true,
    enableSorting: false,
    enableRowSelection: false,
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
  })
  return (
    <div className='flex min-h-0 flex-col gap-4'>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={query.isPending}
        isFetching={query.isFetching}
        emptyTitle={
          query.isError
            ? t('Unable to load donation records')
            : t('No donation records')
        }
        className='h-auto'
        toolbar={
          <div className='flex flex-col gap-3'>
            <DonationRecordFilterBar
              table={table}
              loading={query.isFetching}
              onChange={(next) => {
                const updated = { ...next, p: 1, page_size: filters.page_size }
                if (JSON.stringify(updated) === JSON.stringify(filters)) {
                  void query.refetch()
                } else setFilters(updated)
              }}
            />
            {query.isError && (
              <Alert variant='destructive'>
                <AlertDescription className='flex flex-wrap items-center justify-between gap-3'>
                  <span>{t(query.error.message)}</span>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void query.refetch()}
                  >
                    {t('Retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
          </div>
        }
      />
      {detailId && (
        <DonationRecordDetail
          key={detailId}
          session={props.session}
          itemId={detailId}
          onClose={() => setDetailId(null)}
        />
      )}
    </div>
  )
}
