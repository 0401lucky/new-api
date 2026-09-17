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
import { TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'

import { getBlackroomStatus } from '../api'
import {
  getBlackroomBlockingReasonKey,
  resolveBlackroomGeoReadiness,
} from '../lib'
import type { BlackroomStatusSummary } from '../types'

type StatusItem = {
  key: string
  captionKey: string
  labelKey: string
  variant: StatusVariant
}

const GEO_RESOLVER_BLOCKING_REASON = 'geo_resolver_not_ready'

function buildStatusItems(
  status: BlackroomStatusSummary,
  geoEffective: boolean
): StatusItem[] {
  const items: StatusItem[] = [
    {
      key: 'enabled',
      captionKey: 'Blackroom',
      labelKey: status.enabled ? 'Enabled' : 'Disabled',
      variant: status.enabled ? 'success' : 'neutral',
    },
    {
      key: 'auto_ban',
      captionKey: 'Auto ban',
      labelKey: status.auto_ban_enabled ? 'On' : 'Off',
      variant: status.auto_ban_enabled ? 'success' : 'neutral',
    },
    {
      key: 'realtime',
      captionKey: 'Realtime blocking',
      labelKey: status.realtime_enabled ? 'On' : 'Off',
      variant: status.realtime_enabled ? 'success' : 'neutral',
    },
    {
      key: 'geo',
      captionKey: 'Geo blocking',
      labelKey: geoEffective ? 'Effective' : 'Not effective',
      variant: geoEffective ? 'success' : 'warning',
    },
  ]

  if (status.shadow_mode) {
    items.push({
      key: 'shadow_mode',
      captionKey: 'Shadow mode',
      labelKey: 'Recording only',
      variant: 'warning',
    })
  }

  return items
}

/**
 * 列表页顶部的风控档位摘要：一眼看出是否启用、是否影子模式、地理判定是否
 * 真的生效，以及自动封禁当前被什么挡住。
 */
export function BlackroomStatusBanner() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['blackroom-status'],
    queryFn: getBlackroomStatus,
  })

  const status = data?.data

  if (!status) {
    if (!isLoading) return null
    return <Skeleton className='h-14 w-full rounded-lg' />
  }

  const readiness = resolveBlackroomGeoReadiness(status)
  const items = buildStatusItems(status, readiness.effective)
  const blockingKeys = status.blocking.map(
    (reason) => getBlackroomBlockingReasonKey(reason) ?? reason
  )
  const hasGeoResolverIssue = status.blocking.includes(
    GEO_RESOLVER_BLOCKING_REASON
  )

  return (
    <div className='shrink-0 space-y-2 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center gap-x-4 gap-y-2'>
        {items.map((item) => (
          <div key={item.key} className='flex items-center gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t(item.captionKey)}
            </span>
            <StatusBadge
              label={t(item.labelKey)}
              variant={item.variant}
              copyable={false}
            />
          </div>
        ))}
      </div>

      {blockingKeys.length > 0 && (
        <Alert variant={hasGeoResolverIssue ? 'destructive' : 'default'}>
          <TriangleAlert />
          <AlertTitle>{t('Automatic bans are currently blocked')}</AlertTitle>
          <AlertDescription>
            <ul className='list-disc space-y-0.5 ps-4'>
              {blockingKeys.map((key) => (
                <li key={key}>{t(key)}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
