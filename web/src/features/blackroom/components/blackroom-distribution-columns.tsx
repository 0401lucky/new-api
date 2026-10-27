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

import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getBlackroomDistributionReasonLabel } from '../constants'
import { formatBlackroomTimeValue } from '../lib/blackroom-time'
import type { BlackroomDistributionItem } from '../types'

export function useBlackroomDistributionColumns(options: {
  onSelectUser: (userId: number) => void
  onViewEvidence: (item: BlackroomDistributionItem) => void
}): ColumnDef<BlackroomDistributionItem>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const onSelectUser = options.onSelectUser
  const onViewEvidence = options.onViewEvidence

  // Keep cell components mounted so the dialog can restore focus to its trigger.
  return useMemo<ColumnDef<BlackroomDistributionItem>[]>(
    () => [
      {
        accessorKey: 'username',
        meta: { label: t('User'), mobileTitle: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('User')} />
        ),
        cell: ({ row }) => (
          <Button
            variant='link'
            size='sm'
            onClick={() => onSelectUser(row.original.user_id)}
          >
            {row.original.username ||
              t('User {{id}}', { id: row.original.user_id })}
          </Button>
        ),
      },
      {
        accessorKey: 'score',
        meta: { label: t('Review priority'), mobileBadge: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Review priority')} />
        ),
        cell: ({ row }) => (
          <StatusBadge
            label={`${formatNumber(row.original.score, locale)} / 100`}
            variant={row.original.score > 0 ? 'warning' : 'neutral'}
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
        cell: ({ row }) => formatNumber(row.original.request_count, locale),
      },
      {
        id: 'tokens',
        accessorFn: (item) => item.tokens.length,
        meta: { label: t('API Keys') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('API Keys')} />
        ),
        cell: ({ row }) => formatNumber(row.original.tokens.length, locale),
      },
      {
        id: 'reasons',
        meta: { label: t('Review signals') },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Review signals')} />
        ),
        cell: ({ row }) => (
          <div className='flex max-w-sm flex-wrap gap-1'>
            {row.original.reasons.length === 0 ? (
              <span className='text-muted-foreground text-xs'>
                {t('No review signals')}
              </span>
            ) : (
              row.original.reasons.map((reason) => (
                <StatusBadge
                  key={reason}
                  label={getBlackroomDistributionReasonLabel(reason, t)}
                  variant='neutral'
                  copyable={false}
                />
              ))
            )}
          </div>
        ),
      },
      {
        accessorKey: 'last_seen_at',
        meta: { label: t('Last Seen'), mobileHidden: true },
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Last Seen')} />
        ),
        cell: ({ row }) => formatBlackroomTimeValue(row.original.last_seen_at),
      },
      {
        id: 'evidence',
        meta: { label: t('Evidence') },
        header: t('Evidence'),
        cell: ({ row }) => (
          <Button
            variant='outline'
            size='xs'
            onClick={() => onViewEvidence(row.original)}
          >
            {t('View evidence')}
          </Button>
        ),
      },
    ],
    [locale, onSelectUser, onViewEvidence, t]
  )
}
