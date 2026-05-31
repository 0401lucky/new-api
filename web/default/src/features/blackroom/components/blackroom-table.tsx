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
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type SortingState,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
} from '@/components/data-table'
import { getBlackroom } from '../api'
import {
  getBlackroomSourceOptions,
  getBlackroomStatusOptions,
  normalizeBlackroomStatus,
} from '../constants'
import { useBlackroomColumns } from './blackroom-columns'
import { useBlackroom } from './blackroom-provider'

const route = getRouteApi('/_authenticated/blackroom/')

export function BlackroomTable() {
  const { t } = useTranslation()
  const columns = useBlackroomColumns()
  const { refreshTrigger } = useBlackroom()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'source', searchKey: 'source', type: 'array' },
    ],
  })

  const statusFilter = columnFilters.find((item) => item.id === 'status')
    ?.value as string[] | undefined
  const sourceFilter = columnFilters.find((item) => item.id === 'source')
    ?.value as string[] | undefined

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'blackroom',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter?.join(',') ?? '',
      sourceFilter?.join(',') ?? '',
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getBlackroom({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        filter: globalFilter?.trim() || undefined,
        status: statusFilter?.[0],
        source: sourceFilter?.[0],
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
    state: {
      sorting,
      columnVisibility,
      columnFilters,
      globalFilter,
      pagination,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [pageCount, ensurePageInRange])

  const statusOptions = useMemo(() => getBlackroomStatusOptions(t), [t])
  const sourceOptions = useMemo(() => getBlackroomSourceOptions(t), [t])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No blackroom records found')}
      emptyDescription={t('No users are currently in the blackroom.')}
      skeletonKeyPrefix='blackroom-skeleton'
      toolbarProps={{
        searchPlaceholder: t('Search user, reason, or ID...'),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: statusOptions,
            singleSelect: true,
          },
          {
            columnId: 'source',
            title: t('Source'),
            options: sourceOptions,
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) =>
        normalizeBlackroomStatus(row.original.status) !== 'active'
          ? isMobile
            ? DISABLED_ROW_MOBILE
            : DISABLED_ROW_DESKTOP
          : undefined
      }
      mobileProps={{
        getRowKey: (row) => row.original.id,
      }}
    />
  )
}
