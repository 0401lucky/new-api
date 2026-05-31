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
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { CheckCircle, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PublicLayout } from '@/components/layout'
import { getPublicModelHealthLast24h } from './api'
import type {
  PublicModelHealthHourlyStat,
  PublicModelHealthPayload,
} from './types'
import {
  formatRate,
  formatTokens,
  hourLabel,
  percentileNearestRank,
} from './utils'

type RateLevel = {
  level: 'excellent' | 'good' | 'warning' | 'poor' | 'critical'
  color: string
  bg: string
  text: string
}

function getRateLevel(rate: number): RateLevel {
  const v = Number(rate) || 0
  if (v >= 0.95) {
    return {
      level: 'excellent',
      color: '#4dd0e1',
      bg: 'rgba(77, 208, 225, 0.15)',
      text: 'Excellent',
    }
  }
  if (v >= 0.8) {
    return {
      level: 'good',
      color: '#66bb6a',
      bg: 'rgba(102, 187, 106, 0.15)',
      text: 'Good',
    }
  }
  if (v >= 0.6) {
    return {
      level: 'warning',
      color: '#aed581',
      bg: 'rgba(174, 213, 129, 0.15)',
      text: 'Average',
    }
  }
  if (v >= 0.2) {
    return {
      level: 'poor',
      color: '#ffb74d',
      bg: 'rgba(255, 183, 77, 0.15)',
      text: 'Poor',
    }
  }
  return {
    level: 'critical',
    color: '#ff8a65',
    bg: 'rgba(255, 138, 101, 0.15)',
    text: 'Abnormal',
  }
}

function HealthCell(props: {
  cell: PublicModelHealthHourlyStat
  isLatest: boolean
}) {
  const { t } = useTranslation()
  const rate = Number(props.cell?.success_rate) || 0
  const isFilled = props.cell?.is_filled
  const tokens = Number(props.cell?.success_tokens) || 0
  const { color } = getRateLevel(rate)

  const borderColor = color
  const backgroundColor = 'transparent'
  const opacity = isFilled ? 0.75 : 0.95

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            className='h-[22px] w-[15px] cursor-pointer rounded-[6px] transition-all duration-200 hover:-translate-y-0.5 hover:shadow-sm sm:h-[26px] sm:w-[19px]'
            style={{
              border: '2px solid',
              borderColor,
              borderStyle: 'solid',
              backgroundColor,
              opacity,
            }}
          />
        }
      />
      <TooltipContent className='block max-w-xs p-3 text-xs'>
        <div className='mb-1.5 text-sm font-semibold'>
          {hourLabel(props.cell?.hour_start_ts)}
        </div>
        <div className='space-y-1'>
          <div>
            {t('Success rate')}:{' '}
            <span className='font-medium'>
              {isFilled ? `~${formatRate(rate)}` : formatRate(rate)}
            </span>
          </div>
          <div>
            {t('Total tokens')}:{' '}
            <span className='font-medium'>{formatTokens(tokens)}</span>
          </div>
          {!isFilled && (
            <>
              <div>
                {t('Successful requests')}:{' '}
                <span className='font-medium'>
                  {Number(props.cell?.success_requests) || 0}
                </span>
              </div>
              <div>
                {t('Error')}:{' '}
                <span className='font-medium'>
                  {Number(props.cell?.error_requests) || 0}
                </span>
              </div>
            </>
          )}
          {isFilled && (
            <div className='text-muted-foreground italic'>
              {t('No data, using average value')}
            </div>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

function StatCard(props: {
  title: string
  value: ReactNode
  subtitle?: string
  bgGradient: string
}) {
  return (
    <div
      className='relative flex min-h-[116px] flex-col justify-between overflow-hidden rounded-[20px] border border-white/10 p-5 shadow-lg transition-all duration-300 hover:-translate-y-0.5 hover:shadow-xl sm:rounded-[24px]'
      style={{ background: props.bgGradient }}
    >
      <div className='relative z-10 flex items-center justify-between'>
        <div className='text-sm font-medium tracking-wide text-white/90'>
          {props.title}
        </div>
        <div className='flex h-9 w-9 items-center justify-center rounded-full bg-black/15 shadow-inner'>
          <CheckCircle className='size-5 text-white' strokeWidth={2.5} />
        </div>
      </div>
      <div className='relative z-10 mt-3'>
        <div className='text-2xl font-bold tracking-tight text-white sm:text-3xl'>
          {props.value}
        </div>
        {props.subtitle && (
          <div className='mt-1 text-xs font-medium text-white/80 sm:text-sm'>
            {props.subtitle}
          </div>
        )}
      </div>
    </div>
  )
}

function StatCardSkeleton(props: {
  title: string
  bgGradient: string
  valueWidth?: number
}) {
  return (
    <div
      className='relative flex min-h-[116px] flex-col justify-between overflow-hidden rounded-[20px] border border-white/10 p-5 shadow-lg sm:rounded-[24px]'
      style={{ background: props.bgGradient }}
    >
      <div className='relative z-10 flex items-center justify-between'>
        <div className='text-sm font-medium tracking-wide text-white/90'>
          {props.title}
        </div>
        <div className='flex h-9 w-9 items-center justify-center rounded-full bg-black/15 shadow-inner'>
          <CheckCircle className='size-5 text-white/40' />
        </div>
      </div>
      <div className='relative z-10 mt-3'>
        <Skeleton
          className='mb-2 h-[34px] rounded-[10px] bg-white/35'
          style={{ width: props.valueWidth ?? 110 }}
        />
        <Skeleton className='h-3.5 w-20 rounded-lg bg-white/25' />
      </div>
    </div>
  )
}

function LegendItem(props: { color: string; label: string }) {
  return (
    <div className='flex items-center gap-2 px-1 py-0.5'>
      <div
        className='h-3.5 w-3.5 rounded-full border border-white/10 shadow-sm'
        style={{ backgroundColor: props.color }}
      />
      <span className='text-xs font-medium text-gray-500 dark:text-gray-400'>
        {props.label}
      </span>
    </div>
  )
}

function LegendSkeleton() {
  return (
    <div className='bg-card mb-6 rounded-lg border px-5 py-4 shadow-sm'>
      <div className='flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex flex-wrap items-center gap-3'>
          <Skeleton className='h-3.5 w-[72px] rounded-lg' />
          <div className='flex flex-wrap items-center gap-2'>
            {[86, 92, 78, 96, 82].map((w, idx) => (
              <div
                key={idx}
                className='flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-1.5 dark:bg-gray-800/50'
              >
                <Skeleton className='h-4 w-4 rounded-md' />
                <Skeleton className='h-5 rounded-lg' style={{ width: w }} />
              </div>
            ))}
          </div>
        </div>
        <Skeleton className='h-8 w-[220px] rounded-[10px]' />
      </div>
    </div>
  )
}

function TimeLabelsSkeleton() {
  return (
    <div className='mb-3 overflow-x-auto pl-[200px] sm:pl-[260px]'>
      <div className='flex min-w-max gap-1'>
        {Array.from({ length: 24 }).map((_, idx) => (
          <div
            key={idx}
            className='w-[19px] flex-shrink-0 text-center sm:w-[23px]'
          >
            {idx % 3 === 0 && (
              <Skeleton className='mx-auto h-3 w-[14px] rounded-md' />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function ModelListSkeleton() {
  return (
    <div className='space-y-4'>
      {Array.from({ length: 6 }).map((_, idx) => (
        <div
          key={idx}
          className='bg-card rounded-[20px] border border-gray-100 px-5 py-4 shadow-sm dark:border-gray-800/60'
        >
          <div className='flex items-center gap-4'>
            <div className='w-[180px] flex-shrink-0 sm:w-[240px]'>
              <div className='flex items-center gap-3'>
                <Skeleton className='h-10 w-2.5 rounded-full bg-slate-200 dark:bg-slate-800' />
                <div className='min-w-0 flex-1'>
                  <Skeleton className='mb-2.5 h-4 w-40 rounded-lg' />
                  <div className='flex flex-wrap items-center gap-3'>
                    <Skeleton className='h-[20px] w-[60px] rounded-[6px]' />
                    <Skeleton className='h-3.5 w-[52px] rounded-md' />
                  </div>
                </div>
              </div>
            </div>
            <div className='flex-1 overflow-x-auto'>
              <div className='flex min-w-max gap-1'>
                {Array.from({ length: 24 }).map((__, jdx) => (
                  <div
                    key={jdx}
                    className='h-[22px] w-[15px] animate-pulse rounded-[6px] bg-gray-100 sm:h-[26px] sm:w-[19px] dark:bg-gray-800'
                  />
                ))}
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function LoadingOverlay() {
  return (
    <div className='pointer-events-none fixed inset-x-0 top-20 z-40 flex justify-center'>
      <div className='bg-background/90 flex items-center gap-2 rounded-full border px-4 py-2 text-sm shadow-lg backdrop-blur'>
        <Spinner />
      </div>
    </div>
  )
}

export function ModelHealthPublicPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [errorText, setErrorText] = useState('')
  const [payload, setPayload] = useState<PublicModelHealthPayload | null>(null)
  const [searchText, setSearchText] = useState('')

  async function load(options: { silent?: boolean } = {}) {
    const silent = Boolean(options.silent)
    if (!silent) {
      setLoading(true)
      setErrorText('')
    }
    try {
      const res = await getPublicModelHealthLast24h()
      const { success, message, data } = res || {}
      if (!success) {
        const errMsg = message || t('Load failed')
        if (!silent) {
          setErrorText(errMsg)
          toast.error(errMsg)
        }
        return
      }
      if (!data || typeof data !== 'object') {
        const errMsg = t('Unexpected API response')
        if (!silent) {
          setErrorText(errMsg)
          toast.error(errMsg)
        }
        return
      }
      setPayload(data)
    } catch (error) {
      const errMsg = error instanceof Error ? error.message : t('Load failed')
      if (!silent) {
        setErrorText(t('Load failed'))
        toast.error(errMsg)
      }
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => {
    load().catch(() => undefined)
    const timer = window.setInterval(() => {
      load({ silent: true }).catch(() => undefined)
    }, 30_000)
    return () => window.clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const hourStarts = useMemo(() => {
    const start = Number(payload?.start_hour)
    const end = Number(payload?.end_hour)
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
      return []
    }
    const hours: number[] = []
    for (let ts = start; ts < end; ts += 3600) {
      hours.push(ts)
    }
    return hours
  }, [payload?.start_hour, payload?.end_hour])

  const { modelData, stats } = useMemo(() => {
    const rows = Array.isArray(payload?.rows) ? payload.rows : []
    const byModel = new Map<string, Map<number, PublicModelHealthHourlyStat>>()
    for (const row of rows) {
      const name = row?.model_name || ''
      if (!name) continue
      if (!byModel.has(name)) byModel.set(name, new Map())
      byModel.get(name)?.set(Number(row.hour_start_ts), row)
    }

    const models = Array.from(byModel.keys())
    const totalModels = models.length
    let healthyModels = 0
    let warningModels = 0
    let criticalModels = 0
    let totalSuccessSlices = 0
    let totalSlices = 0
    let totalSuccessTokens = 0

    const modelData = models.map((modelName) => {
      const hourMap = byModel.get(modelName)
      let modelTotalSuccess = 0
      let modelTotalSlices = 0
      let modelTotalTokens = 0

      const hourlyTokens = hourStarts.map(
        (ts) => Number(hourMap?.get(ts)?.success_tokens) || 0
      )
      const p10Tokens = percentileNearestRank(
        hourlyTokens.filter((token) => token > 0),
        0.1
      )

      for (const ts of hourStarts) {
        const stat = hourMap?.get(ts)
        const hourTokens = Number(stat?.success_tokens) || 0
        const hasData = Boolean(stat && Number(stat.total_slices) > 0)
        const isLowTraffic = p10Tokens > 0 && hourTokens < p10Tokens
        if (hasData && !isLowTraffic) {
          modelTotalSuccess += Number(stat?.success_slices) || 0
          modelTotalSlices += Number(stat?.total_slices) || 0
        }
        modelTotalTokens += hourTokens
      }

      const avgRate =
        modelTotalSlices > 0 ? modelTotalSuccess / modelTotalSlices : 0
      totalSuccessSlices += modelTotalSuccess
      totalSlices += modelTotalSlices
      totalSuccessTokens += modelTotalTokens

      const { level } = getRateLevel(avgRate)
      if (level === 'excellent' || level === 'good') healthyModels++
      else if (level === 'warning') warningModels++
      else if (level === 'critical') criticalModels++

      const hourlyData = hourStarts.map((ts) => {
        const stat = hourMap?.get(ts)
        const hourTokens = Number(stat?.success_tokens) || 0
        const hasData = Boolean(stat && Number(stat.total_slices) > 0)
        const isLowTraffic = p10Tokens > 0 && hourTokens < p10Tokens
        if (stat && hasData && !isLowTraffic) return stat
        return {
          hour_start_ts: ts,
          model_name: modelName,
          success_slices: 0,
          total_slices: 0,
          success_rate: avgRate,
          total_requests: 0,
          error_requests: 0,
          success_requests: 0,
          success_tokens: hourTokens,
          is_filled: true,
        }
      })

      return {
        model_name: modelName,
        avg_rate: avgRate,
        total_success: modelTotalSuccess,
        total_slices: modelTotalSlices,
        total_tokens: modelTotalTokens,
        hourly: hourlyData.reverse(),
      }
    })

    modelData.sort((a, b) => (b.total_tokens || 0) - (a.total_tokens || 0))
    const overallRate = totalSlices > 0 ? totalSuccessSlices / totalSlices : 0

    return {
      modelData,
      stats: {
        totalModels,
        healthyModels,
        warningModels,
        criticalModels,
        overallRate,
        totalSuccessSlices,
        totalSlices,
        totalSuccessTokens,
      },
    }
  }, [payload?.rows, hourStarts])

  const filteredModelData = useMemo(() => {
    if (!searchText.trim()) return modelData
    const keyword = searchText.toLowerCase().trim()
    return modelData.filter((m) => m.model_name.toLowerCase().includes(keyword))
  }, [modelData, searchText])

  const latestHour =
    hourStarts.length > 0 ? hourStarts[hourStarts.length - 1] : null
  const isInitialLoading = loading && !payload
  const showSpin = loading && Boolean(payload)

  return (
    <PublicLayout showMainContainer={false}>
      <TooltipProvider>
        <div className='mx-auto mt-[60px] max-w-6xl px-3 pb-10 sm:px-6 lg:px-8'>
          <div className='mb-8'>
            <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <h1 className='text-xl font-bold tracking-tight text-gray-800 sm:text-2xl dark:text-gray-100'>
                  {t('Model health overview for the last 24 hours')}
                </h1>
                <p className='mt-1.5 text-xs text-gray-500 sm:text-sm dark:text-gray-500'>
                  {t(
                    'Model health status for the last 24 hours, monitoring all requests (including errors caused by malformed requests)'
                  )}
                </p>
              </div>
            </div>
          </div>

          {errorText && (
            <div className='mb-6 rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-600'>
              {errorText}
            </div>
          )}

          {showSpin && <LoadingOverlay />}

          <div className='mb-8 grid grid-cols-2 gap-4 sm:gap-5 lg:grid-cols-4'>
            {isInitialLoading ? (
              <>
                <StatCardSkeleton
                  title={t('Monitored models')}
                  bgGradient='linear-gradient(135deg, #2ec4b6 0%, #0ea5e9 100%)'
                  valueWidth={72}
                />
                <StatCardSkeleton
                  title={t('Overall success rate')}
                  bgGradient='linear-gradient(135deg, #4caf50 0%, #8bc34a 100%)'
                  valueWidth={96}
                />
                <StatCardSkeleton
                  title={t('Total tokens')}
                  bgGradient='linear-gradient(135deg, #2563eb 0%, #3b82f6 100%)'
                  valueWidth={120}
                />
                <StatCardSkeleton
                  title={t('Healthy models')}
                  bgGradient='linear-gradient(135deg, #14b8a6 0%, #10b981 100%)'
                  valueWidth={72}
                />
              </>
            ) : (
              <>
                <StatCard
                  title={t('Monitored models')}
                  value={stats.totalModels}
                  subtitle={t('{{count}} healthy', {
                    count: stats.healthyModels,
                  })}
                  bgGradient='linear-gradient(135deg, #2ec4b6 0%, #0ea5e9 100%)'
                />
                <StatCard
                  title={t('Overall success rate')}
                  value={formatRate(stats.overallRate)}
                  subtitle={t('Past 24 hours')}
                  bgGradient='linear-gradient(135deg, #4caf50 0%, #8bc34a 100%)'
                />
                <StatCard
                  title={t('Total tokens')}
                  value={formatTokens(stats.totalSuccessTokens)}
                  subtitle={t('Past 24 hours')}
                  bgGradient='linear-gradient(135deg, #2563eb 0%, #3b82f6 100%)'
                />
                <StatCard
                  title={t('Healthy models')}
                  value={stats.healthyModels}
                  subtitle={t('Success rate >= 80%')}
                  bgGradient='linear-gradient(135deg, #14b8a6 0%, #10b981 100%)'
                />
              </>
            )}
          </div>

          {isInitialLoading ? (
            <LegendSkeleton />
          ) : (
            <div className='mb-6 border-b border-gray-200/70 pb-5 dark:border-gray-800/80'>
              <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
                <div className='flex flex-wrap items-center gap-3.5'>
                  <span className='mr-1.5 text-sm font-semibold text-gray-700 dark:text-gray-300'>
                    {t('Status legend')}
                  </span>
                  <div className='flex flex-wrap items-center gap-3'>
                    <LegendItem
                      color='#4dd0e1'
                      label={t('Excellent (>=95%)')}
                    />
                    <LegendItem color='#66bb6a' label={t('Good (80-95%)')} />
                    <LegendItem color='#aed581' label={t('Average (60-80%)')} />
                    <LegendItem color='#ffb74d' label={t('Poor (20-60%)')} />
                    <LegendItem color='#ff8a65' label={t('Abnormal (<20%)')} />
                  </div>
                </div>
                <div className='relative w-full sm:w-[220px]'>
                  <Search className='pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-gray-400' />
                  <input
                    className='focus:bg-background focus:ring-primary/20 h-9 w-full rounded-full border border-transparent bg-gray-100/70 pr-8 pl-9.5 text-xs transition-all outline-none focus:border-gray-200 focus:ring-2 dark:bg-gray-800/60 dark:focus:border-gray-700'
                    placeholder={t('Search models...')}
                    value={searchText}
                    onChange={(event) => setSearchText(event.target.value)}
                  />
                  {searchText && (
                    <button
                      type='button'
                      className='text-muted-foreground hover:text-foreground absolute top-1/2 right-2.5 -translate-y-1/2 rounded p-1'
                      onClick={() => setSearchText('')}
                      aria-label={t('Clear search')}
                    >
                      <X className='size-3.5' />
                    </button>
                  )}
                </div>
              </div>
            </div>
          )}

          {isInitialLoading ? (
            <TimeLabelsSkeleton />
          ) : (
            hourStarts.length > 0 && (
              <div className='mb-3 overflow-x-auto pl-[200px] sm:pl-[260px]'>
                <div className='flex min-w-max gap-1'>
                  {[...hourStarts].reverse().map((ts, idx) => (
                    <div
                      key={ts}
                      className='w-[19px] flex-shrink-0 text-center sm:w-[23px]'
                    >
                      {idx % 3 === 0 && (
                        <div className='text-[11px] font-medium text-gray-400'>
                          {hourLabel(ts)}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )
          )}

          {isInitialLoading ? (
            <ModelListSkeleton />
          ) : (
            <div className='space-y-4'>
              {filteredModelData.map((item) => {
                const { color, bg } = getRateLevel(item.avg_rate)
                return (
                  <div
                    key={item.model_name}
                    className='bg-white dark:bg-slate-900 rounded-[20px] border border-gray-200/40 px-5 py-4 transition-all duration-300 hover:border-gray-300/80 hover:shadow-md dark:border-gray-800 dark:hover:border-gray-700'
                  >
                    <div className='flex items-center gap-4'>
                      <div className='w-[180px] flex-shrink-0 sm:w-[240px]'>
                        <div className='flex items-center gap-3'>
                          <div
                            className='h-10 w-2.5 flex-shrink-0 rounded-full shadow-sm'
                            style={{ backgroundColor: color }}
                          />
                          <div className='min-w-0 flex-1'>
                            <Tooltip>
                              <TooltipTrigger className='hover:text-primary block w-full truncate text-left text-sm font-semibold text-gray-800 transition-colors sm:text-base dark:text-gray-100'>
                                {item.model_name}
                              </TooltipTrigger>
                              <TooltipContent className='break-all'>
                                {item.model_name}
                              </TooltipContent>
                            </Tooltip>
                            <div className='mt-1.5 flex flex-wrap items-center gap-3 text-xs sm:text-sm'>
                              <span
                                className='rounded-[6px] px-2 py-0.5 text-xs font-bold'
                                style={{ color, backgroundColor: bg }}
                              >
                                {formatRate(item.avg_rate)}
                              </span>
                              <span className='font-medium text-gray-400 dark:text-gray-500'>
                                {formatTokens(item.total_tokens)}
                              </span>
                            </div>
                          </div>
                        </div>
                      </div>

                      <div className='flex-1 overflow-x-auto'>
                        <div className='flex min-w-max gap-1'>
                          {item.hourly.map((cell) => (
                            <HealthCell
                              key={cell.hour_start_ts}
                              cell={cell}
                              isLatest={cell.hour_start_ts === latestHour}
                            />
                          ))}
                        </div>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}

          {!loading && !isInitialLoading && filteredModelData.length === 0 && (
            <div className='bg-card rounded-lg border shadow-sm'>
              <div className='py-16 text-center'>
                <div className='mb-6 text-7xl'>📊</div>
                <h4 className='text-muted-foreground text-lg font-semibold'>
                  {searchText ? t('No matching models found') : t('No data')}
                </h4>
                <p className='text-muted-foreground mt-2 text-base'>
                  {searchText
                    ? t('Try another search keyword')
                    : t('Please refresh and try again later')}
                </p>
              </div>
            </div>
          )}
        </div>
      </TooltipProvider>
    </PublicLayout>
  )
}
