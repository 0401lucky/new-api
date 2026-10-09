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
import { getRouteApi } from '@tanstack/react-router'
import type { RowSelectionState, SortingState } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getGroups, getUserActivity } from '../api'
import { getUserRoleOptions, getUserStatusOptions } from '../constants'
import type { GetUserActivityParams, User, UserActivityFilter } from '../types'
import { UserActivityBulkActions } from './user-activity-bulk-actions'
import { useUserActivityColumns } from './user-activity-columns'
import { UserActivitySummary } from './user-activity-summary'
import { useUsers } from './users-provider'

const route = getRouteApi('/_authenticated/users/')
const emptyUsers: User[] = []
const emptySelection: RowSelectionState = {}

export function UserActivityPanel() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const users = useUsers()
  const columns = useUserActivityColumns()
  const [sorting, setSorting] = useState<SortingState>([
    { id: 'last_request_at', desc: false },
  ])
  const [selection, setSelection] = useState({ key: '', rows: emptySelection })
  const url = useTableUrlState({
    search,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'role', searchKey: 'role', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
    ],
  })
  const filters = Object.fromEntries(
    url.columnFilters.map((filter) => [
      filter.id,
      (filter.value as string[])[0],
    ])
  )
  const activity: UserActivityFilter = search.activity ?? ''
  const params: GetUserActivityParams = {
    p: url.pagination.pageIndex + 1,
    page_size: url.pagination.pageSize,
    keyword: url.globalFilter,
    group: filters.group,
    role: filters.role,
    status: filters.status,
    activity,
    sort_by:
      (sorting[0]?.id as GetUserActivityParams['sort_by']) ?? 'last_request_at',
    sort_order: sorting[0]?.desc ? 'desc' : 'asc',
  }
  const query = useQuery({
    queryKey: ['users', 'activity', params, users.refreshTrigger],
    queryFn: () => getUserActivity(params),
  })
  const groups = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    staleTime: 5 * 60 * 1000,
  })

  // Selection belongs to one result page. Filter changes, pagination, refreshes
  // and background updates must not carry hidden accounts into a deletion.
  const selectionKey = JSON.stringify([params, query.dataUpdatedAt])
  const rowSelection =
    selection.key === selectionKey ? selection.rows : emptySelection
  const { table } = useDataTable({
    data: query.data?.items ?? emptyUsers,
    columns,
    getRowId: (user) => String(user.id),
    enableRowSelection: (row) =>
      !query.isFetching && row.original.cleanup_eligible === true,
    rowSelection,
    onRowSelectionChange: (updater) =>
      setSelection((previous) => ({
        key: selectionKey,
        rows:
          typeof updater === 'function'
            ? updater(
                previous.key === selectionKey ? previous.rows : emptySelection
              )
            : updater,
      })),
    sorting,
    onSortingChange: (updater) => {
      setSorting(updater)
      url.onPaginationChange({ ...url.pagination, pageIndex: 0 })
    },
    pagination: url.pagination,
    onPaginationChange: url.onPaginationChange,
    globalFilter: url.globalFilter,
    onGlobalFilterChange: url.onGlobalFilterChange,
    columnFilters: url.columnFilters,
    onColumnFiltersChange: url.onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    totalCount: query.data?.total ?? 0,
    ensurePageInRange: url.ensurePageInRange,
    columnVisibilityStorageKey: 'user-activity-column-visibility',
    columnSizingStorageKey: 'user-activity-column-sizing',
  })

  const changeActivity = (value: UserActivityFilter) => {
    void navigate({
      search: (previous) => ({
        ...previous,
        activity: value || undefined,
        page: 1,
      }),
    })
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <UserActivitySummary
        summary={query.data?.summary}
        activity={activity}
        onActivityChange={changeActivity}
        onRefresh={() => void query.refetch()}
        isFetching={query.isFetching}
      />
      {query.isError ? (
        <ErrorState
          title={t('Failed to load user statistics')}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <DataTablePage
          className='min-h-0 flex-1'
          table={table}
          columns={columns}
          isLoading={query.isLoading}
          isFetching={query.isFetching}
          emptyTitle={t('No Users Found')}
          emptyDescription={t(
            'No users available. Try adjusting your search or filters.'
          )}
          skeletonKeyPrefix='user-activity-skeleton'
          applyHeaderSize
          toolbarProps={{
            searchPlaceholder: t(
              'Filter by username, name, email; use #ID for exact user ID'
            ),
            searchDebounceMs: 500,
            filters: [
              {
                columnId: 'status',
                title: t('Status'),
                options: getUserStatusOptions(t).filter(
                  (option) => option.value !== '-1'
                ),
                singleSelect: true,
              },
              {
                columnId: 'role',
                title: t('Role'),
                options: getUserRoleOptions(t),
                singleSelect: true,
              },
              {
                columnId: 'group',
                title: t('User Group'),
                options: (groups.data?.data ?? []).map((group) => ({
                  label: group,
                  value: group,
                })),
                singleSelect: true,
              },
            ],
          }}
          bulkActions={
            <UserActivityBulkActions
              table={table}
              disabled={query.isFetching}
            />
          }
          mobileProps={{ enableRowSelection: true }}
          showMobileBulkActions
        />
      )}
    </div>
  )
}
