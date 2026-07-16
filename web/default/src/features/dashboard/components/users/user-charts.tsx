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
import {
  useEffect,
  useMemo,
  useState,
  useRef,
  useCallback,
  type ReactNode,
} from 'react'
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { Users, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { formatNumber, formatQuota } from '@/lib/format'
import { getRollingDateRange, type TimeGranularity } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'
import { useTheme } from '@/context/theme-provider'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import {
  getUserModelUsageStats,
  getUserQuotaDataByUsers,
} from '@/features/dashboard/api'
import {
  TIME_GRANULARITY_OPTIONS,
  TIME_RANGE_PRESETS,
} from '@/features/dashboard/constants'
import {
  getDefaultDays,
  saveGranularity,
  processUserChartData,
} from '@/features/dashboard/lib'
import type {
  ProcessedUserChartData,
  QuotaDataItem,
  UserChartsFilters,
  UserModelUsageResponse,
} from '@/features/dashboard/types'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

const USER_CHARTS: {
  value: string
  labelKey: string
  specKey: keyof ProcessedUserChartData
}[] = [
  {
    value: 'rank',
    labelKey: 'User Consumption Ranking',
    specKey: 'spec_user_rank',
  },
  {
    value: 'trend',
    labelKey: 'User Consumption Trend',
    specKey: 'spec_user_trend',
  },
]

const TOP_USER_LIMIT_OPTIONS = [5, 10, 20, 50]

interface SelectedUser {
  user_id: number
  username: string
}

function getChartDatum(event: unknown): Record<string, unknown> | null {
  if (!event || typeof event !== 'object') return null
  const eventRecord = event as Record<string, unknown>
  const candidates = [
    eventRecord.datum,
    (eventRecord.data as Record<string, unknown> | undefined)?.datum,
    (eventRecord.params as Record<string, unknown> | undefined)?.datum,
  ]
  const datum = candidates.find(
    (item) => item && typeof item === 'object' && !Array.isArray(item)
  )
  return (datum as Record<string, unknown> | undefined) ?? null
}

function getChartText(event: unknown): string {
  if (!event || typeof event !== 'object') return ''
  const eventRecord = event as Record<string, unknown>
  const target = eventRecord.target as Record<string, unknown> | undefined
  const targetAttribute = target?.attribute as
    | Record<string, unknown>
    | undefined
  const targetAttributes = target?.attributes as
    | Record<string, unknown>
    | undefined
  const value =
    eventRecord.value ??
    eventRecord.text ??
    eventRecord.label ??
    targetAttribute?.text ??
    targetAttributes?.text
  return typeof value === 'string' || typeof value === 'number'
    ? String(value)
    : ''
}

function formatTimeRange(range: {
  start_timestamp: number
  end_timestamp: number
}) {
  const start = new Date(range.start_timestamp * 1000).toLocaleString()
  const end = new Date(range.end_timestamp * 1000).toLocaleString()
  return `${start} - ${end}`
}

function MetricCard(props: { title: string; value: string }) {
  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardDescription>{props.title}</CardDescription>
        <CardTitle className='text-2xl'>{props.value}</CardTitle>
      </CardHeader>
    </Card>
  )
}

function UserModelUsageSheet(props: {
  open: boolean
  user: SelectedUser | null
  timeRange: { start_timestamp: number; end_timestamp: number }
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [
      'dashboard',
      'user-model-usage',
      props.user?.user_id,
      props.timeRange,
    ],
    queryFn: () =>
      getUserModelUsageStats({
        user_id: props.user?.user_id ?? 0,
        start_timestamp: props.timeRange.start_timestamp,
        end_timestamp: props.timeRange.end_timestamp,
        limit: 1000,
      }),
    enabled: props.open && Boolean(props.user?.user_id),
    staleTime: 30_000,
  })

  const usage: UserModelUsageResponse | undefined = query.data?.success
    ? query.data.data
    : undefined
  const rows = usage?.models ?? []
  const maxRequests = rows.reduce(
    (max, row) => Math.max(max, Number(row.request_count) || 0),
    0
  )

  let tableRows: ReactNode
  if (query.isFetching && rows.length === 0) {
    tableRows = [
      'model-loading-1',
      'model-loading-2',
      'model-loading-3',
      'model-loading-4',
      'model-loading-5',
      'model-loading-6',
    ].map((key) => (
      <TableRow key={key}>
        <TableCell>
          <Skeleton className='h-4 w-56' />
          <Skeleton className='mt-2 h-1.5 w-full' />
        </TableCell>
        <TableCell>
          <Skeleton className='ml-auto h-4 w-16' />
        </TableCell>
        <TableCell>
          <Skeleton className='ml-auto h-4 w-20' />
        </TableCell>
        <TableCell>
          <Skeleton className='ml-auto h-4 w-20' />
        </TableCell>
      </TableRow>
    ))
  } else if (rows.length === 0) {
    tableRows = (
      <TableRow>
        <TableCell colSpan={4}>
          <Empty className='min-h-56 border-0'>
            <EmptyHeader>
              <EmptyTitle>{t('No data')}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        </TableCell>
      </TableRow>
    )
  } else {
    tableRows = rows.map((row) => {
      const requests = Number(row.request_count) || 0
      const percent =
        maxRequests > 0
          ? Math.max(4, Math.round((requests / maxRequests) * 100))
          : 0
      return (
        <TableRow key={row.model_name || 'unknown'}>
          <TableCell className='max-w-[28rem]'>
            <div className='flex min-w-0 flex-col gap-2'>
              <div className='flex min-w-0 items-center gap-2'>
                <span className='truncate font-medium'>
                  {row.model_name || '-'}
                </span>
                {requests === maxRequests && maxRequests > 0 ? (
                  <Badge variant='secondary'>{t('Top')}</Badge>
                ) : null}
              </div>
              <div className='bg-muted h-1.5 overflow-hidden rounded-full'>
                <div
                  className='bg-primary h-full rounded-full'
                  style={{ width: `${percent}%` }}
                />
              </div>
            </div>
          </TableCell>
          <TableCell className='text-right font-medium'>
            {formatNumber(row.request_count)}
          </TableCell>
          <TableCell className='text-right'>
            {formatNumber(row.total_tokens)}
          </TableCell>
          <TableCell className='text-right'>
            {formatQuota(Number(row.quota) || 0)}
          </TableCell>
        </TableRow>
      )
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-4xl')}>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('User Model Usage')}</SheetTitle>
          <SheetDescription>
            {props.user
              ? `${props.user.username} · #${props.user.user_id} · ${formatTimeRange(props.timeRange)}`
              : t('Loading')}
          </SheetDescription>
        </SheetHeader>

        <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-4 py-4 sm:px-6'>
          <div className='grid gap-3 sm:grid-cols-3'>
            <MetricCard
              title={t('Total requests')}
              value={formatNumber(usage?.total_requests)}
            />
            <MetricCard
              title={t('Total tokens')}
              value={formatNumber(usage?.total_tokens)}
            />
            <MetricCard
              title={t('Total quota')}
              value={usage ? formatQuota(usage.total_quota) : '-'}
            />
          </div>

          <Card className='min-h-0 flex-1 gap-0 overflow-hidden py-0'>
            <CardHeader className='border-b py-3'>
              <div className='flex items-center justify-between gap-3'>
                <div className='min-w-0'>
                  <CardTitle className='text-base'>
                    {t('Model call details')}
                  </CardTitle>
                  <CardDescription>
                    {t('{{count}} models', { count: rows.length })}
                  </CardDescription>
                </div>
                {query.isFetching && (
                  <Loader2 className='text-muted-foreground size-4 animate-spin' />
                )}
              </div>
            </CardHeader>
            <CardContent className='min-h-0 flex-1 overflow-auto p-0'>
              <Table className='min-w-[680px]'>
                <TableHeader className='bg-background sticky top-0 z-10'>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead className='w-32 text-right'>
                      {t('Requests')}
                    </TableHead>
                    <TableHead className='w-36 text-right'>
                      {t('Tokens')}
                    </TableHead>
                    <TableHead className='w-36 text-right'>
                      {t('Quota')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>{tableRows}</TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </SheetContent>
    </Sheet>
  )
}

interface UserChartsProps {
  filters: UserChartsFilters
  onFiltersChange: (filters: UserChartsFilters) => void
}

export function UserCharts(props: UserChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const [selectedUser, setSelectedUser] = useState<SelectedUser | null>(null)
  const [usageOpen, setUsageOpen] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)

  // The selection is owned by the dashboard parent so it persists across
  // sub-section switches; the rolling window is derived from the chosen range.
  const timeGranularity = props.filters.timeGranularity
  const selectedRange = props.filters.selectedRange
  const topUserLimit = props.filters.topUserLimit
  const onFiltersChange = props.onFiltersChange

  const timeRange = useMemo(() => {
    const { start, end } = getRollingDateRange(selectedRange)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
    }
  }, [selectedRange])

  const handleRangeChange = useCallback(
    (days: number) => {
      onFiltersChange({ ...props.filters, selectedRange: days })
    },
    [onFiltersChange, props.filters]
  )

  const handleGranularityChange = useCallback(
    (g: TimeGranularity) => {
      saveGranularity(g)
      onFiltersChange({
        ...props.filters,
        timeGranularity: g,
        selectedRange: getDefaultDays(g),
      })
    },
    [onFiltersChange, props.filters]
  )

  const handleTopUserLimitChange = useCallback(
    (limit: number) => {
      onFiltersChange({ ...props.filters, topUserLimit: limit })
    },
    [onFiltersChange, props.filters]
  )

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    updateTheme()
  }, [resolvedTheme])

  const { data: userData, isLoading } = useQuery({
    queryKey: ['dashboard', 'user-quota', timeRange],
    queryFn: () => getUserQuotaDataByUsers(timeRange),
    select: (res) => (res.success ? res.data : []),
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      processUserChartData(
        isLoading ? [] : (userData ?? []),
        timeGranularity,
        t,
        topUserLimit
      ),
    [userData, isLoading, timeGranularity, t, topUserLimit]
  )

  const userIndex = useMemo(() => {
    const map = new Map<string, SelectedUser & { quota: number }>()
    const sourceData = (userData ?? []) as QuotaDataItem[]
    sourceData.forEach((item) => {
      const username = item.username || 'unknown'
      const userId = Number(item.user_id) || 0
      if (userId <= 0) return
      const prev = map.get(username)
      map.set(username, {
        user_id: prev?.user_id ?? userId,
        username,
        quota: (prev?.quota ?? 0) + (Number(item.quota) || 0),
      })
    })
    return map
  }, [userData])

  const handleChartClick = useCallback(
    (event: unknown) => {
      const datum = getChartDatum(event)
      const username = String(datum?.User || getChartText(event))
      if (!username) return
      const user = userIndex.get(username)
      if (!user) return
      setSelectedUser({ user_id: user.user_id, username: user.username })
      setUsageOpen(true)
    },
    [userIndex]
  )

  return (
    <>
      <div className='space-y-3'>
        <div className='flex items-center gap-1.5 overflow-x-auto pb-1 sm:gap-2'>
          <Tabs
            value={String(selectedRange)}
            onValueChange={(value) => handleRangeChange(Number(value))}
            className='shrink-0'
          >
            <TabsList>
              {TIME_RANGE_PRESETS.map((preset) => (
                <TabsTrigger
                  key={preset.days}
                  value={String(preset.days)}
                  className='px-2.5 text-xs'
                >
                  {t(preset.label)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          <Tabs
            value={timeGranularity}
            onValueChange={(value) =>
              handleGranularityChange(value as TimeGranularity)
            }
            className='shrink-0'
          >
            <TabsList>
              {TIME_GRANULARITY_OPTIONS.map((opt) => (
                <TabsTrigger
                  key={opt.value}
                  value={opt.value}
                  className='px-2.5 text-xs'
                >
                  {t(opt.label)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          <Tabs
            value={String(topUserLimit)}
            onValueChange={(value) => handleTopUserLimitChange(Number(value))}
            className='shrink-0'
          >
            <TabsList>
              <span className='text-muted-foreground px-2 text-xs font-medium whitespace-nowrap'>
                {t('Top Users')}
              </span>
              {TOP_USER_LIMIT_OPTIONS.map((limit) => (
                <TabsTrigger
                  key={limit}
                  value={String(limit)}
                  className='px-2.5 text-xs'
                >
                  {t('Top {{count}}', { count: limit })}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          {isLoading && (
            <Loader2 className='text-muted-foreground size-4 animate-spin' />
          )}
        </div>

        <div className='grid gap-3'>
          {USER_CHARTS.map((chart) => {
            const spec = chartData[chart.specKey]

            return (
              <div
                key={chart.value}
                className='overflow-hidden rounded-lg border'
              >
                <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
                  <IconBadge tone='info' size='sm'>
                    <Users />
                  </IconBadge>
                  <div className='text-sm font-semibold'>
                    {t(chart.labelKey)}
                  </div>
                </div>

                <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
                  {isLoading ? (
                    <Skeleton className='h-full w-full' />
                  ) : (
                    themeReady &&
                    spec && (
                      <VChart
                        key={`user-${chart.value}-${topUserLimit}-${resolvedTheme}`}
                        spec={{
                          ...spec,
                          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                          background: 'transparent',
                        }}
                        onClick={handleChartClick}
                        option={VCHART_OPTION}
                      />
                    )
                  )}
                </div>
              </div>
            )
          })}
        </div>
      </div>
      <UserModelUsageSheet
        open={usageOpen}
        user={selectedUser}
        timeRange={timeRange}
        onOpenChange={setUsageOpen}
      />
    </>
  )
}
