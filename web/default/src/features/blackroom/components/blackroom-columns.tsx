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
import { type ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  BLACKROOM_SOURCES,
  BLACKROOM_STATUSES,
  normalizeBlackroomSource,
  normalizeBlackroomStatus,
} from '../constants'
import type { BlackroomEntry } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

function formatTimeValue(value: unknown): string {
  if (typeof value === 'number') {
    return formatTimestampToDate(value)
  }
  if (typeof value === 'string' && value.trim()) {
    const numeric = Number(value)
    if (Number.isFinite(numeric)) {
      return formatTimestampToDate(numeric)
    }
    return value
  }
  return '-'
}

function formatHours(seconds: number | null | undefined, t: TFunction) {
  if (!seconds || seconds <= 0) return ''
  const hours = seconds / 3600
  if (Number.isInteger(hours)) {
    return t('{{hours}} hours', { hours })
  }
  return t('{{hours}} hours', { hours: hours.toFixed(1) })
}

function formatBanDuration(entry: BlackroomEntry, t: TFunction) {
  if (entry.banned_until === 0) {
    return t('Permanent')
  }
  const duration = formatHours(entry.ban_duration_seconds, t)
  const until = formatTimeValue(entry.banned_until)
  if (duration && until !== '-') {
    return `${duration} / ${until}`
  }
  return duration || until
}

function formatTerminalTime(entry: BlackroomEntry, t: TFunction) {
  const status = normalizeBlackroomStatus(entry.status)
  if (status === 'released') {
    return formatTimeValue(entry.released_at)
  }
  if (status === 'expired') {
    return formatTimeValue(entry.banned_until)
  }
  if (entry.banned_until === 0) {
    return t('Permanent')
  }
  return formatTimeValue(entry.banned_until)
}

export function useBlackroomColumns(): ColumnDef<BlackroomEntry>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('ID')} />
      ),
      cell: ({ row }) => (
        <TableId value={row.getValue('id') as number} className='w-[60px]' />
      ),
    },
    {
      id: 'user',
      accessorFn: (row) => row.username || String(row.user_id),
      meta: { label: t('User'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: ({ row }) => {
        const entry = row.original
        return (
          <div className='min-w-[120px]'>
            <div className='truncate font-medium'>
              {entry.username || t('User {{id}}', { id: entry.user_id })}
            </div>
            <div className='text-muted-foreground text-xs'>
              {t('User ID:')} {entry.user_id}
            </div>
          </div>
        )
      },
    },
    {
      accessorKey: 'status',
      meta: { label: t('Status'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const status = normalizeBlackroomStatus(row.getValue('status'))
        const config = BLACKROOM_STATUSES[status] ?? {
          labelKey: String(row.getValue('status') ?? '-'),
          variant: 'neutral' as const,
        }
        return (
          <StatusBadge
            label={t(config.labelKey)}
            variant={config.variant}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) =>
        value.includes(normalizeBlackroomStatus(row.getValue(id))),
    },
    {
      accessorKey: 'source',
      meta: { label: t('Source'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Source')} />
      ),
      cell: ({ row }) => {
        const source = normalizeBlackroomSource(row.getValue('source'))
        const config = BLACKROOM_SOURCES[source] ?? {
          labelKey: String(row.getValue('source') ?? '-'),
          variant: 'neutral' as const,
        }
        return (
          <StatusBadge
            label={t(config.labelKey)}
            variant={config.variant}
            copyable={false}
          />
        )
      },
      filterFn: (row, id, value) =>
        value.includes(normalizeBlackroomSource(row.getValue(id))),
    },
    {
      accessorKey: 'ip_count',
      meta: { label: t('IP Count') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Count')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatNumber(row.getValue('ip_count'))}
        </span>
      ),
    },
    {
      id: 'duration',
      meta: { label: t('Ban Duration / Until') },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Ban Duration / Until')}
        />
      ),
      cell: ({ row }) => (
        <span className='min-w-[140px] font-mono text-sm'>
          {formatBanDuration(row.original, t)}
        </span>
      ),
    },
    {
      id: 'terminal_time',
      meta: { label: t('Release / Expire Time'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('Release / Expire Time')}
        />
      ),
      cell: ({ row }) => (
        <div className='min-w-[140px] font-mono text-sm'>
          {formatTerminalTime(row.original, t)}
        </div>
      ),
    },
    {
      accessorKey: 'reason',
      meta: { label: t('Reason') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Reason')} />
      ),
      cell: ({ row }) => (
        <div className='max-w-[240px] truncate text-sm'>
          {(row.getValue('reason') as string) || '-'}
        </div>
      ),
      enableSorting: false,
    },
    {
      accessorKey: 'created_at',
      meta: { label: t('Created'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-[140px] font-mono text-sm'>
          {formatTimeValue(row.getValue('created_at'))}
        </div>
      ),
    },
    {
      id: 'actions',
      cell: ({ row }) => <DataTableRowActions row={row} />,
    },
  ]
}
