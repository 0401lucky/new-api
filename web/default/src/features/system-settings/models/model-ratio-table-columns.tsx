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
import { Plus } from 'lucide-react'

import { DataTableColumnHeader } from '@/components/data-table/core/column-header'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'

import {
  getModeLabel,
  getModeVariant,
  getPriceDetail,
  getPriceSummary,
  type ChannelModelStatus,
  type ModelRow,
} from './model-pricing-snapshots'

const filterBySelectedValues = (
  rowValue: unknown,
  filterValue: unknown
): boolean => {
  if (!Array.isArray(filterValue) || filterValue.length === 0) return true
  return filterValue.includes(String(rowValue))
}

const CHANNEL_STATUS_META: Record<
  ChannelModelStatus,
  { label: string; variant: 'success' | 'warning' | 'neutral' }
> = {
  in_channel: { label: 'In channel', variant: 'success' },
  unpriced: { label: 'Unpriced', variant: 'warning' },
  not_in_channel: { label: 'Not in channel', variant: 'neutral' },
}

type BuildModelRatioColumnsOptions = {
  onDelete: (name: string) => void
  onEdit: (model: ModelRow) => void
  showChannelStatus: boolean
  deleteDisabled?: boolean
  t: (key: string) => string
}

export function buildModelRatioColumns({
  onDelete,
  onEdit,
  showChannelStatus,
  deleteDisabled,
  t,
}: BuildModelRatioColumnsOptions): ColumnDef<ModelRow>[] {
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('Select all')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) =>
        row.original.isUnpriced ? null : (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label={t('Select row')}
            className='translate-y-[2px]'
          />
        ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    },
    {
      accessorKey: 'name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model name')} />
      ),
      cell: ({ row }) => (
        <div className='flex min-w-0 items-center gap-2 font-medium'>
          <span className='min-w-0 truncate'>{row.getValue('name')}</span>
          {row.original.billingMode === 'tiered_expr' && (
            <StatusBadge
              label={t('Tiered')}
              variant='info'
              copyable={false}
              className='shrink-0'
            />
          )}
          {row.original.hasConflict && (
            <StatusBadge
              label={t('Conflict')}
              variant='danger'
              copyable={false}
              className='shrink-0'
            />
          )}
        </div>
      ),
      enableHiding: false,
    },
    {
      accessorKey: 'billingMode',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Mode')} />
      ),
      cell: ({ row }) =>
        row.original.isUnpriced ? null : (
          <StatusBadge
            label={t(getModeLabel(row.original.billingMode))}
            variant={getModeVariant(row.original.billingMode)}
            copyable={false}
            showDot={false}
            className='-ml-1.5 px-0'
          />
        ),
      filterFn: (row, id, value) =>
        filterBySelectedValues(row.getValue(id), value),
      meta: { label: t('Mode') },
    },
    ...(showChannelStatus
      ? [
          {
            accessorKey: 'channelStatus',
            header: ({ column }) => (
              <DataTableColumnHeader column={column} title={t('Channel')} />
            ),
            cell: ({ row }) => {
              const status = row.original.channelStatus
              if (!status) return null
              const meta = CHANNEL_STATUS_META[status]
              return (
                <StatusBadge
                  label={t(meta.label)}
                  variant={meta.variant}
                  copyable={false}
                  showDot={false}
                  className='-ml-1.5 px-0'
                />
              )
            },
            filterFn: (row, id, value) =>
              filterBySelectedValues(row.getValue(id), value),
            meta: { label: t('Channel') },
          } as ColumnDef<ModelRow>,
        ]
      : []),
    {
      id: 'priceSummary',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Price summary')} />
      ),
      cell: ({ row }) =>
        row.original.isUnpriced ? (
          <span className='text-muted-foreground'>—</span>
        ) : (
          <div className='flex min-w-0 flex-col gap-1'>
            <span className='truncate font-medium'>
              {getPriceSummary(row.original, t)}
            </span>
            <span className='text-muted-foreground truncate text-xs'>
              {getPriceDetail(row.original, t)}
            </span>
          </div>
        ),
      sortingFn: (rowA, rowB) =>
        getPriceSummary(rowA.original, t).localeCompare(
          getPriceSummary(rowB.original, t)
        ),
      meta: { label: t('Price summary') },
    },
    {
      id: 'actions',
      header: () => <div>{t('Actions')}</div>,
      cell: ({ row }) =>
        row.original.isUnpriced ? (
          <Button
            variant='outline'
            size='sm'
            onClick={() => onEdit(row.original)}
          >
            <Plus data-icon='inline-start' />
            {t('Add pricing')}
          </Button>
        ) : (
          <StaticRowActions
            editLabel={t('Edit')}
            deleteLabel={t('Delete')}
            menuLabel={t('Open menu')}
            onEdit={() => onEdit(row.original)}
            onDelete={() => onDelete(row.original.name)}
            deleteDisabled={deleteDisabled}
          />
        ),
      enableHiding: false,
    },
  ]
}
