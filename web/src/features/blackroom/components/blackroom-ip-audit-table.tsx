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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage } from '@/components/data-table'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import type { NavigateFn } from '@/hooks/use-table-url-state'

import { getBlackroomIPAudit } from '../api'
import { useBlackroomIPAuditSearchState } from '../hooks/use-ip-audit-search-state'
import type { BlackroomIPAuditSearchParams } from '../types'
import { useBlackroomIPAuditColumns } from './blackroom-ip-audit-columns'
import { useBlackroom } from './blackroom-provider'

export function BlackroomIPAuditTable(props: {
  search: BlackroomIPAuditSearchParams
  navigate: NavigateFn
  onSelectUser: (userId: number) => void
}) {
  const { t } = useTranslation()
  const { refreshTrigger } = useBlackroom()
  const {
    pagination,
    globalFilter,
    range,
    onGlobalFilterChange,
    onRangeChange,
    onPaginationChange,
    ensurePageInRange,
  } = useBlackroomIPAuditSearchState({
    search: props.search,
    navigate: props.navigate,
  })

  const columns = useBlackroomIPAuditColumns({
    onSelectUser: props.onSelectUser,
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'blackroom-ip-audit',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      range.start?.getTime() ?? 0,
      range.end?.getTime() ?? 0,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getBlackroomIPAudit({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        filter: globalFilter.trim() || undefined,
        start_at: Math.floor((range.start?.getTime() ?? 0) / 1000) || undefined,
        end_at: Math.floor((range.end?.getTime() ?? 0) / 1000) || undefined,
      })

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const table = useReactTable({
    data: data?.items || [],
    columns,
    state: { pagination, globalFilter },
    onPaginationChange,
    onGlobalFilterChange,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No IP observations found')}
      emptyDescription={t(
        'No requests were observed for the selected filters.'
      )}
      skeletonKeyPrefix='blackroom-ip-audit-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Search IP address...'),
        additionalSearch: (
          <CompactDateTimeRangePicker
            start={range.start}
            end={range.end}
            onChange={onRangeChange}
            className='w-full sm:w-[260px]'
          />
        ),
        hasAdditionalFilters: Boolean(range.start || range.end),
        onReset: () => onRangeChange({}),
      }}
      mobileProps={{
        getRowKey: (row) => row.original.ip,
      }}
    />
  )
}
