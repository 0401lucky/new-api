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
import type { ColumnDef, ColumnFiltersState } from '@tanstack/react-table'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTableColumnHeader,
  DataTablePagination,
  DataTableToolbar,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

interface ModelUsageRow {
  model: string
  requests: number
  tokens: number
}

interface ModelUsageTableProps {
  data: QuotaDataItem[]
  loading?: boolean
  error?: boolean
}

const EMPTY_COLUMN_FILTERS: ColumnFiltersState = []

export function ModelUsageTable(props: ModelUsageTableProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [search, setSearch] = useState('')
  const data = useMemo(() => {
    const models = new Map<string, ModelUsageRow>()
    for (const item of props.data) {
      const model = item.model_name ?? ''
      const row = models.get(model) ?? { model, requests: 0, tokens: 0 }
      row.requests += Number(item.count) || 0
      row.tokens += Number(item.token_used) || 0
      models.set(model, row)
    }
    return [...models.values()]
  }, [props.data])
  const columns = useMemo<ColumnDef<ModelUsageRow>[]>(
    () => [
      {
        accessorKey: 'model',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Model')} />
        ),
        cell: ({ row }) => (
          <span className='block max-w-96 font-mono text-xs break-all whitespace-normal'>
            {row.original.model || t('Unknown')}
          </span>
        ),
        enableHiding: false,
      },
      {
        accessorKey: 'requests',
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Requests')}
            className='justify-end'
          />
        ),
        cell: ({ row }) => (
          <span className='block text-right tabular-nums'>
            {formatNumber(row.original.requests, locale)}
          </span>
        ),
        enableGlobalFilter: false,
        enableHiding: false,
      },
      {
        accessorKey: 'tokens',
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Total Tokens')}
            className='justify-end'
          />
        ),
        cell: ({ row }) => (
          <span className='block text-right tabular-nums'>
            {formatNumber(row.original.tokens, locale)}
          </span>
        ),
        enableGlobalFilter: false,
        enableHiding: false,
      },
    ],
    [t, locale]
  )
  const { table } = useDataTable({
    data,
    columns,
    getRowId: (row) => row.model,
    initialSorting: [{ id: 'requests', desc: true }],
    initialPagination: { pageIndex: 0, pageSize: 10 },
    globalFilter: search,
    columnFilters: EMPTY_COLUMN_FILTERS,
    onGlobalFilterChange: setSearch,
    globalFilterFn: 'includesString',
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
    enableRowSelection: false,
  })

  return (
    <section
      className='min-w-0 space-y-3 rounded-lg border p-3 sm:p-5'
      aria-label={t('Model Usage Details')}
      aria-busy={props.loading}
    >
      <div>
        <h2 className='text-sm font-semibold'>{t('Model Usage Details')}</h2>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Requests and tokens by model for the selected filters.')}
        </p>
      </div>
      {props.error ? (
        <ErrorState />
      ) : (
        <>
          <DataTableToolbar
            table={table}
            searchPlaceholder={t('Search models...')}
            hideViewOptions
          />
          <DataTableView
            table={table}
            isLoading={props.loading}
            tableClassName='min-w-[480px]'
            tableContainerClassName='overflow-x-auto'
          />
          <DataTablePagination table={table} compact />
        </>
      )}
    </section>
  )
}
