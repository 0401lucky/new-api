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
import { Gauge, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getRateLimitUsage, type RateLimitMetric } from './api'

function UsageMeter(props: { label: string; metric: RateLimitMetric }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const limited = props.metric.limit > 0 && props.metric.used !== null
  const used = props.metric.used ?? 0
  const percentage = limited
    ? Math.min(100, (used / props.metric.limit) * 100)
    : 0

  return (
    <Progress
      value={percentage}
      aria-valuetext={
        limited
          ? `${formatNumber(used, locale)} / ${formatNumber(props.metric.limit, locale)}`
          : t('Unlimited')
      }
      className={cn(
        'gap-y-2 [&_[data-slot=progress-track]]:h-2',
        limited &&
          used >= props.metric.limit &&
          '[&_[data-slot=progress-indicator]]:bg-destructive'
      )}
    >
      <div className='flex w-full flex-wrap items-baseline justify-between gap-x-4 gap-y-1'>
        <ProgressLabel className='text-muted-foreground min-w-0 font-normal'>
          {props.label}
        </ProgressLabel>
        <span className='text-sm font-medium tabular-nums'>
          {limited
            ? `${formatNumber(used, locale)} / ${formatNumber(props.metric.limit, locale)}`
            : t('Unlimited')}
        </span>
      </div>
    </Progress>
  )
}

export function RateLimitUsage() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const userId = useAuthStore((state) => state.auth.user?.id)
  const query = useQuery({
    queryKey: ['self-rate-limit-usage', userId],
    queryFn: ({ signal }) => getRateLimitUsage(signal),
    enabled: Boolean(userId),
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
    retry: false,
    meta: { errorToast: false },
  })
  const data = query.data

  let content
  if (query.isPending) {
    content = <LoadingState />
  } else if (query.isError || !data) {
    content = (
      <ErrorState
        title={t('Failed to load rate limit usage')}
        onRetry={() => void query.refetch()}
      />
    )
  } else if (!data.enabled || data.exempt) {
    content = (
      <EmptyState
        icon={Gauge}
        title={
          data.exempt
            ? t('Your account is exempt from user rate limits')
            : t('User rate limits are disabled')
        }
        description={t('Custom API key limits still apply.')}
      />
    )
  } else if (data.groups.length === 0) {
    content = <EmptyState title={t('No available groups')} />
  } else {
    content = (
      <div className='space-y-5'>
        <div className='space-y-1.5'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='text-base font-semibold'>
              {t('My rate limit usage')}
            </h3>
            <Badge variant='secondary'>
              {t('Window: {{minutes}} min', {
                minutes: formatNumber(data.window_minutes, locale),
              })}
            </Badge>
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Updates every 5 seconds. Usage is shared across groups; each group has its own limits.'
            )}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Only enabled limits are counted. Custom API key limits apply separately.'
            )}
          </p>
        </div>
        <div className='grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-3'>
          {data.groups.map((group) => (
            <Card key={group.group} className='min-w-0'>
              <CardHeader>
                <CardTitle className='break-all'>{group.group}</CardTitle>
                <CardDescription>{t('User rate limits')}</CardDescription>
              </CardHeader>
              <CardContent className='space-y-5'>
                <UsageMeter
                  label={t('Successful requests / {{minutes}} min', {
                    minutes: formatNumber(data.window_minutes, locale),
                  })}
                  metric={group.success}
                />
                {group.total.limit > 0 && (
                  <UsageMeter
                    label={
                      data.total_mode === 'token_bucket'
                        ? t('Total request limit usage')
                        : t('Total requests / {{minutes}} min', {
                            minutes: formatNumber(data.window_minutes, locale),
                          })
                    }
                    metric={group.total}
                  />
                )}
                <UsageMeter
                  label={t('Concurrent requests')}
                  metric={group.concurrency}
                />
              </CardContent>
            </Card>
          ))}
        </div>
        {data.total_mode === 'token_bucket' &&
          data.groups.some((group) => group.total.limit > 0) && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Total request capacity recovers continuously; its usage is not a rolling request count.'
              )}
            </p>
          )}
        {data.total_mode === 'sliding_window' && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'Successful request usage includes in-flight reservations until those requests finish.'
            )}
          </p>
        )}
      </div>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Rate limit usage')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw
            data-icon='inline-start'
            aria-hidden='true'
            className={cn(query.isFetching && 'animate-spin')}
          />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
