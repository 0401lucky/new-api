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
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useTableUrlState, type NavigateFn } from '@/hooks/use-table-url-state'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getBlackroomDistribution } from '../api'
import type {
  BlackroomDistributionItem,
  BlackroomDistributionSearchParams,
} from '../types'
import { useBlackroomDistributionColumns } from './blackroom-distribution-columns'
import { BlackroomDistributionDialog } from './blackroom-distribution-dialog'
import { useBlackroom } from './blackroom-provider'

const EMPTY_ITEMS: BlackroomDistributionItem[] = []

export function BlackroomDistributionTable(props: {
  search: BlackroomDistributionSearchParams
  navigate: NavigateFn
  onSelectUser: (userId: number) => void
}) {
  const { t } = useTranslation()
  const { refreshTrigger } = useBlackroom()
  const [selected, setSelected] = useState<BlackroomDistributionItem | null>(
    null
  )
  const {
    pagination,
    globalFilter,
    onPaginationChange,
    onGlobalFilterChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: props.search,
    navigate: props.navigate,
    pagination: {
      pageKey: 'distPage',
      pageSizeKey: 'distPageSize',
      defaultPageSize: 20,
    },
    globalFilter: { key: 'distFilter' },
  })
  const query = useQuery({
    queryKey: [
      'blackroom-distribution',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const response = requireServerSuccess(
        await getBlackroomDistribution({
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
          filter: globalFilter?.trim() || undefined,
        })
      )
      if (!response.data) {
        throw new Error(t('Unable to load distribution observations'))
      }
      return response.data
    },
    placeholderData: (previous) => previous,
    refetchInterval: 30_000,
    meta: { errorToast: false },
  })
  const columns = useBlackroomDistributionColumns({
    onSelectUser: props.onSelectUser,
    onViewEvidence: setSelected,
  })
  const table = useReactTable({
    data: query.data?.items ?? EMPTY_ITEMS,
    columns,
    defaultColumn: { enableSorting: false },
    state: { pagination, globalFilter },
    onPaginationChange,
    onGlobalFilterChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: Math.ceil((query.data?.total ?? 0) / pagination.pageSize),
  })
  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  if (query.isError) {
    return (
      <ErrorState
        title={t('Unable to load distribution observations')}
        onRetry={() => {
          void query.refetch()
        }}
      />
    )
  }
  const unavailable = query.data && !query.data.available
  let emptyDescription = t(
    'Validated model requests appear here as observations are collected.'
  )
  if (unavailable) {
    emptyDescription =
      query.data.unavailable_reason === 'blackroom_disabled'
        ? t('Enable Blackroom to collect observations.')
        : t('Redis is required to collect distribution observations.')
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <Alert role='status'>
        <AlertTitle>{t('Redistribution watchlist')}</AlertTitle>
        <AlertDescription>
          {t(
            'Scores prioritize review. They do not prove redistribution or trigger bans.'
          )}{' '}
          {t(
            'Prompt metrics cover the latest minute. Network and client metrics cover the latest 10 minutes.'
          )}
        </AlertDescription>
      </Alert>
      {query.data?.scope_limited && (
        <Alert>
          <AlertDescription>
            {t(
              'The observation limit was reached. Some active keys are omitted from this ranking.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className='min-h-0 flex-1'>
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={query.isLoading}
          isFetching={query.isFetching}
          emptyTitle={
            unavailable
              ? t('Observations unavailable')
              : t('No distribution observations found')
          }
          emptyDescription={emptyDescription}
          skeletonKeyPrefix='blackroom-distribution-skeleton'
          toolbarProps={{
            searchPlaceholder: t('Search username or user ID...'),
          }}
          mobileProps={{ getRowKey: (row) => String(row.original.user_id) }}
        />
      </div>
      <BlackroomDistributionDialog
        item={selected}
        onClose={() => setSelected(null)}
        onSelectUser={props.onSelectUser}
      />
    </div>
  )
}
