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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { TimestampCell } from '@/components/activity-time-cell'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { User, UserActivity } from '../types'
import { useUsersColumns } from './users-columns'

export function useUserActivityColumns(): ColumnDef<User>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userColumns = useUsersColumns()

  return useMemo(() => {
    const labels: Record<UserActivity, string> = {
      active: t('Active users'),
      inactive: t('Inactive users'),
      very_inactive: t('Very inactive users'),
      never_requested: t('Never requested'),
      unknown: t('Incomplete history'),
    }
    const identityColumns = userColumns
      .filter(
        (column) =>
          column.id === 'select' ||
          ('accessorKey' in column &&
            ['id', 'username', 'status', 'role', 'group'].includes(
              String(column.accessorKey)
            ))
      )
      .map((column) => {
        if ('accessorKey' in column && column.accessorKey === 'group') {
          return { ...column, enableSorting: false }
        }
        return column
      })
    const activityColumns: ColumnDef<User>[] = [
      {
        accessorKey: 'activity',
        header: t('Request activity'),
        cell: ({ row }) => labels[row.original.activity ?? 'unknown'],
        enableSorting: false,
        size: 160,
        meta: { mobileOrder: 35 },
      },
      {
        accessorKey: 'last_request_at',
        header: t('Last request'),
        cell: ({ row }) => {
          if (!row.original.last_request_at) {
            return (
              <span className='text-muted-foreground'>
                {labels[row.original.activity ?? 'unknown']}
              </span>
            )
          }
          return (
            <TimestampCell
              timestamp={row.original.last_request_at}
              format='absolute'
              justNowLabel={t('Just now')}
            />
          )
        },
        size: 200,
        meta: { mobileOrder: 40 },
      },
      {
        accessorKey: 'no_request_days',
        header: t('Days without requests'),
        cell: ({ row }) => {
          const days = formatNumber(row.original.no_request_days ?? 0, locale)
          return row.original.activity === 'unknown' ? `≥ ${days}` : days
        },
        enableSorting: false,
        size: 170,
        meta: { mobileOrder: 45 },
      },
      {
        accessorKey: 'created_at',
        header: t('Registration time'),
        cell: ({ row }) => (
          <TimestampCell
            timestamp={row.original.created_at ?? 0}
            format='absolute'
            justNowLabel={t('Just now')}
          />
        ),
        size: 200,
        meta: { mobileOrder: 50 },
      },
    ]
    return [...identityColumns, ...activityColumns]
  }, [userColumns, t, locale])
}
