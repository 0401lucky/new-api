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
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatNumber } from '@/lib/format'

import { formatBlackroomTimeValue } from '../lib/blackroom-time'
import type { BlackroomIPAuditItem } from '../types'

export function useBlackroomIPAuditColumns(options: {
  onSelectUser: (userId: number) => void
}): ColumnDef<BlackroomIPAuditItem>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'ip',
      meta: { label: t('IP Address'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Address')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>{row.original.ip}</span>
      ),
    },
    {
      accessorKey: 'user_count',
      meta: { label: t('Users'), mobileBadge: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Users')} />
      ),
      cell: ({ row }) => (
        <StatusBadge
          label={formatNumber(row.original.user_count)}
          variant={row.original.user_count > 1 ? 'warning' : 'neutral'}
          copyable={false}
        />
      ),
    },
    {
      accessorKey: 'request_count',
      meta: { label: t('Requests') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Requests')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatNumber(row.original.request_count)}
        </span>
      ),
    },
    {
      id: 'users',
      meta: { label: t('Related users') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Related users')} />
      ),
      cell: ({ row }) => {
        const item = row.original
        const remaining = item.user_count - item.users.length

        return (
          <div className='flex max-w-[260px] flex-wrap items-center gap-x-2 gap-y-1'>
            {item.users.map((user) => (
              <Button
                key={user.user_id}
                type='button'
                variant='link'
                size='xs'
                className='h-auto p-0 text-xs'
                onClick={() => options.onSelectUser(user.user_id)}
              >
                {user.username || t('User {{id}}', { id: user.user_id })}
              </Button>
            ))}
            {item.users_truncated && remaining > 0 && (
              <span className='text-muted-foreground text-xs'>
                {t('+{{count}} more', { count: remaining })}
              </span>
            )}
          </div>
        )
      },
      enableSorting: false,
    },
    {
      accessorKey: 'last_seen_at',
      meta: { label: t('Last Seen') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Last Seen')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-[140px] font-mono text-sm'>
          {formatBlackroomTimeValue(row.original.last_seen_at)}
        </div>
      ),
    },
    {
      accessorKey: 'first_seen_at',
      meta: { label: t('First Seen'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('First Seen')} />
      ),
      cell: ({ row }) => (
        <div className='min-w-[140px] font-mono text-sm'>
          {formatBlackroomTimeValue(row.original.first_seen_at)}
        </div>
      ),
    },
  ]
}
