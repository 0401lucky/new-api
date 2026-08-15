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
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Activity, CheckCircle, RefreshCw, Search, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PublicLayout } from '@/components/layout'
import { cn } from '@/lib/utils'
import { getPublicModelHealthOverview } from './api'
import { ModelHealthCard } from './components/model-health-card'
import { GLOBAL_STATUS_META } from './status'
import type { ModelHealthOverviewPayload, ModelHealthPeriod } from './types'
import { formatRate, formatTokens, timestamp2string } from './utils'

const REFRESH_INTERVAL_MS = 30_000

const PERIOD_LABEL_KEYS: Record<ModelHealthPeriod, string> = {
  '7d': '7 days',
  '15d': '15 days',
  '30d': '30 days',
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
  const [payload, setPayload] = useState<ModelHealthOverviewPayload | null>(
    null
  )
  const [searchText, setSearchText] = useState('')
  const [period, setPeriod] = useState<ModelHealthPeriod>('7d')
  const [nextRefreshAt, setNextRefreshAt] = useState(
    () => Date.now() + REFRESH_INTERVAL_MS
  )
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL_MS / 1000)

  const load = useCallback(
    async (options: { silent?: boolean } = {}) => {
      const silent = Boolean(options.silent)
      if (!silent) {
        setLoading(true)
        setErrorText('')
      }
      try {
        const res = await getPublicModelHealthOverview(period)
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
        setNextRefreshAt(Date.now() + REFRESH_INTERVAL_MS)
      } catch (error) {
        const errMsg = error instanceof Error ? error.message : t('Load failed')
        if (!silent) {
          setErrorText(t('Load failed'))
          toast.error(errMsg)
        }
      } finally {
        if (!silent) setLoading(false)
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [period]
  )

  useEffect(() => {
    load().catch(() => undefined)
    const timer = window.setInterval(() => {
      load({ silent: true }).catch(() => undefined)
    }, REFRESH_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [load])

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCountdown(Math.max(0, Math.ceil((nextRefreshAt - Date.now()) / 1000)))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [nextRefreshAt])

  const filteredModels = useMemo(() => {
    const list = Array.isArray(payload?.models) ? payload.models : []
    if (!searchText.trim()) return list
    const keyword = searchText.toLowerCase().trim()
    return list.filter((m) => m.model_name.toLowerCase().includes(keyword))
  }, [payload?.models, searchText])

  const stats = payload?.stats
  const globalMeta = payload ? GLOBAL_STATUS_META[payload.global_status] : null
  const periodLabel = t(PERIOD_LABEL_KEYS[period])
  const isInitialLoading = loading && !payload
  const showSpin = loading && Boolean(payload)

  return (
    <PublicLayout showMainContainer={false}>
      <TooltipProvider>
        <div className='mx-auto mt-[60px] max-w-6xl px-3 pb-10 sm:px-6 lg:px-8'>
          <div className='mb-8 flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between'>
            <div>
              <div className='text-muted-foreground mb-3 flex items-center gap-2 text-xs font-bold tracking-[0.2em] uppercase'>
                <Activity className='size-4' />
                {t('System status')}
              </div>
              <h1 className='text-4xl leading-[1.05] font-extrabold tracking-tight sm:text-5xl'>
                {t('Model health')}
              </h1>
              <p className='text-muted-foreground mt-3 max-w-xl text-sm'>
                {t(
                  'Real-time availability, latency and trends aggregated from live traffic.'
                )}
              </p>
            </div>
            <div className='flex flex-col items-start gap-2 lg:items-end'>
              {payload && globalMeta && (
                <span
                  className={cn(
                    'inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold',
                    globalMeta.badgeClass
                  )}
                >
                  <span
                    className={cn(
                      'size-2 animate-pulse rounded-full',
                      globalMeta.dotClass
                    )}
                  />
                  {t(globalMeta.labelKey)}
                </span>
              )}
              <div className='text-muted-foreground flex items-center gap-2 text-xs'>
                <RefreshCw className='size-3' />
                {payload && (
                  <span>
                    {t('Updated at {{time}}', {
                      time: timestamp2string(payload.updated_at),
                    })}
                  </span>
                )}
                <span className='opacity-60'>·</span>
                <span>{t('Refresh in {{seconds}}s', { seconds: countdown })}</span>
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
            {isInitialLoading || !stats ? (
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
                  value={stats.total_models}
                  subtitle={t('{{count}} healthy', {
                    count: stats.healthy_models,
                  })}
                  bgGradient='linear-gradient(135deg, #2ec4b6 0%, #0ea5e9 100%)'
                />
                <StatCard
                  title={t('Overall success rate')}
                  value={formatRate(stats.overall_rate_24h)}
                  subtitle={t('Past 24 hours')}
                  bgGradient='linear-gradient(135deg, #4caf50 0%, #8bc34a 100%)'
                />
                <StatCard
                  title={t('Total tokens')}
                  value={formatTokens(stats.total_tokens_24h)}
                  subtitle={t('Past 24 hours')}
                  bgGradient='linear-gradient(135deg, #2563eb 0%, #3b82f6 100%)'
                />
                <StatCard
                  title={t('Healthy models')}
                  value={stats.healthy_models}
                  subtitle={t('Status operational')}
                  bgGradient='linear-gradient(135deg, #14b8a6 0%, #10b981 100%)'
                />
              </>
            )}
          </div>

          <div className='mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
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
            <ToggleGroup
              variant='outline'
              size='sm'
              value={[period]}
              onValueChange={(value: string[]) => {
                const next = value[0] as ModelHealthPeriod | undefined
                if (next && next !== period) setPeriod(next)
              }}
              aria-label={t('Availability')}
            >
              {(Object.keys(PERIOD_LABEL_KEYS) as ModelHealthPeriod[]).map(
                (p) => (
                  <ToggleGroupItem
                    key={p}
                    value={p}
                    aria-label={t(PERIOD_LABEL_KEYS[p])}
                  >
                    {t(PERIOD_LABEL_KEYS[p])}
                  </ToggleGroupItem>
                )
              )}
            </ToggleGroup>
          </div>

          {isInitialLoading ? (
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3'>
              {Array.from({ length: 6 }).map((_, idx) => (
                <Skeleton key={idx} className='h-[280px] rounded-xl' />
              ))}
            </div>
          ) : (
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3'>
              {filteredModels.map((m) => (
                <ModelHealthCard
                  key={m.model_name}
                  model={m}
                  periodLabel={periodLabel}
                />
              ))}
            </div>
          )}

          {payload && filteredModels.length === 0 && (
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
