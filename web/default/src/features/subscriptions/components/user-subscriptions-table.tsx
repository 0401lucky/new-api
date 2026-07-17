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
import { Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useDebounce } from '@/hooks/use-debounce'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getAdminPlans, listAdminUserSubscriptions } from '../api'
import { formatTimestamp } from '../lib'
import {
  getEffectiveSubscriptionStatus,
  getSubscriptionUsagePercent,
} from '../lib/subscription-status'
import type { AdminUserSubscriptionItem } from '../types'
import { useSubscriptions } from './subscriptions-provider'

const PAGE_SIZE = 20

function StatusCell({
  item,
  t,
}: {
  item: AdminUserSubscriptionItem
  t: (key: string) => string
}) {
  const status = getEffectiveSubscriptionStatus(item.subscription)
  if (status === 'active') {
    return <StatusBadge label={t('Active')} variant='success' copyable={false} />
  }
  if (status === 'cancelled') {
    return (
      <StatusBadge label={t('Invalidated')} variant='neutral' copyable={false} />
    )
  }
  return <StatusBadge label={t('Expired')} variant='neutral' copyable={false} />
}

function QuotaUsageCell({
  item,
  t,
}: {
  item: AdminUserSubscriptionItem
  t: (key: string) => string
}) {
  const total = Number(item.subscription.amount_total || 0)
  const used = Number(item.subscription.amount_used || 0)
  if (total <= 0) {
    return (
      <div className='text-sm'>
        <div className='text-muted-foreground'>{t('Unlimited')}</div>
        <div className='text-xs'>
          {t('Used')}: {formatQuota(used)}
        </div>
      </div>
    )
  }
  const pct = getSubscriptionUsagePercent(used, total) ?? 0
  const remaining = Math.max(0, total - used)
  return (
    <div className='min-w-[140px] space-y-1'>
      <div className='flex justify-between gap-2 text-xs'>
        <span>
          {formatQuota(used)} / {formatQuota(total)}
        </span>
        <span className='text-muted-foreground'>{pct.toFixed(0)}%</span>
      </div>
      <Progress value={pct} className='h-1.5' />
      <div className='text-muted-foreground text-xs'>
        {t('Remaining')}: {formatQuota(remaining)}
      </div>
    </div>
  )
}

export function UserSubscriptionsTable() {
  const { t } = useTranslation()
  const { refreshTrigger } = useSubscriptions()
  const [page, setPage] = useState(1)
  const [keywordInput, setKeywordInput] = useState('')
  const keyword = useDebounce(keywordInput, 300)
  const [status, setStatus] = useState<string>('all')
  const [planId, setPlanId] = useState<string>('all')
  const [source, setSource] = useState<string>('all')

  useEffect(() => {
    setPage(1)
  }, [keyword, status, planId, source])

  const { data: plansData } = useQuery({
    queryKey: ['admin-subscription-plans', refreshTrigger],
    queryFn: async () => {
      const res = await getAdminPlans()
      return res.data || []
    },
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-user-subscriptions',
      page,
      PAGE_SIZE,
      keyword,
      status,
      planId,
      source,
      refreshTrigger,
    ],
    queryFn: async () => {
      const res = await listAdminUserSubscriptions({
        p: page,
        page_size: PAGE_SIZE,
        keyword: keyword.trim() || undefined,
        status: status === 'all' ? undefined : status,
        plan_id: planId === 'all' ? undefined : Number(planId),
        source: source === 'all' ? undefined : source,
      })
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const items = data?.items || []
  const total = data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const planOptions = useMemo(
    () =>
      (plansData || []).map((p) => ({
        value: String(p.plan.id),
        label: p.plan.title || `#${p.plan.id}`,
      })),
    [plansData]
  )

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center'>
        <div className='relative min-w-[200px] flex-1'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2' />
          <Input
            value={keywordInput}
            onChange={(e) => setKeywordInput(e.target.value)}
            placeholder={t('Search user / plan / ID')}
            className='pl-8'
          />
        </div>
        <Select
          items={[
            { value: 'all', label: t('All statuses') },
            { value: 'active', label: t('Active') },
            { value: 'expired', label: t('Expired') },
            { value: 'cancelled', label: t('Invalidated') },
          ]}
          value={status}
          onValueChange={(v) => v && setStatus(v)}
        >
          <SelectTrigger className='w-full sm:w-[150px]'>
            <SelectValue placeholder={t('Status')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              <SelectItem value='active'>{t('Active')}</SelectItem>
              <SelectItem value='expired'>{t('Expired')}</SelectItem>
              <SelectItem value='cancelled'>{t('Invalidated')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          items={[
            { value: 'all', label: t('All plans') },
            ...planOptions,
          ]}
          value={planId}
          onValueChange={(v) => v && setPlanId(v)}
        >
          <SelectTrigger className='w-full sm:w-[180px]'>
            <SelectValue placeholder={t('Plan')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All plans')}</SelectItem>
              {planOptions.map((p) => (
                <SelectItem key={p.value} value={p.value}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          items={[
            { value: 'all', label: t('All sources') },
            { value: 'order', label: t('Purchase') },
            { value: 'admin', label: t('Admin') },
            { value: 'auto_grant', label: t('Auto-grant') },
            { value: 'balance', label: t('Balance') },
          ]}
          value={source}
          onValueChange={(v) => v && setSource(v)}
        >
          <SelectTrigger className='w-full sm:w-[150px]'>
            <SelectValue placeholder={t('Source')} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All sources')}</SelectItem>
              <SelectItem value='order'>{t('Purchase')}</SelectItem>
              <SelectItem value='admin'>{t('Admin')}</SelectItem>
              <SelectItem value='auto_grant'>{t('Auto-grant')}</SelectItem>
              <SelectItem value='balance'>{t('Balance')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <div
        className={cn(
          'min-h-0 flex-1 overflow-auto rounded-md border',
          isFetching && !isLoading && 'opacity-80'
        )}
      >
        <table className='w-full min-w-[900px] text-sm'>
          <thead className='bg-muted/50 sticky top-0 z-10'>
            <tr className='border-b text-left'>
              <th className='px-3 py-2 font-medium'>{t('ID')}</th>
              <th className='px-3 py-2 font-medium'>{t('User')}</th>
              <th className='px-3 py-2 font-medium'>{t('Plan')}</th>
              <th className='px-3 py-2 font-medium'>{t('Status')}</th>
              <th className='px-3 py-2 font-medium'>{t('Quota usage')}</th>
              <th className='px-3 py-2 font-medium'>{t('Validity')}</th>
              <th className='px-3 py-2 font-medium'>{t('Source')}</th>
            </tr>
          </thead>
          <tbody>
            {isLoading && items.length === 0 ? (
              <tr>
                <td
                  colSpan={7}
                  className='text-muted-foreground px-3 py-10 text-center'
                >
                  {t('Loading...')}
                </td>
              </tr>
            ) : items.length === 0 ? (
              <tr>
                <td
                  colSpan={7}
                  className='text-muted-foreground px-3 py-10 text-center'
                >
                  {t('No subscription records')}
                </td>
              </tr>
            ) : (
              items.map((item) => {
                const sub = item.subscription
                return (
                  <tr key={sub.id} className='border-b last:border-0'>
                    <td className='px-3 py-2'>
                      <TableId value={sub.id} />
                    </td>
                    <td className='px-3 py-2'>
                      <div className='font-medium'>
                        {item.username || '-'}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        ID: {sub.user_id}
                      </div>
                    </td>
                    <td className='px-3 py-2'>
                      <div className='font-medium'>
                        {item.plan_title || `#${sub.plan_id}`}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        plan #{sub.plan_id}
                      </div>
                    </td>
                    <td className='px-3 py-2'>
                      <StatusCell item={item} t={t} />
                    </td>
                    <td className='px-3 py-2'>
                      <QuotaUsageCell item={item} t={t} />
                    </td>
                    <td className='px-3 py-2 text-xs'>
                      <div>
                        {t('Start')}: {formatTimestamp(sub.start_time)}
                      </div>
                      <div>
                        {t('End')}: {formatTimestamp(sub.end_time)}
                      </div>
                      {sub.next_reset_time ? (
                        <div className='text-muted-foreground'>
                          {t('Next reset')}:{' '}
                          {formatTimestamp(sub.next_reset_time)}
                        </div>
                      ) : null}
                    </td>
                    <td className='px-3 py-2 text-xs'>
                      {sub.source || '-'}
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>

      <div className='flex items-center justify-between gap-2'>
        <div className='text-muted-foreground text-xs'>
          {t('Total {{count}} records', { count: total })}
        </div>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={page <= 1 || isFetching}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {page} / {totalPages}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={page >= totalPages || isFetching}
            onClick={() => setPage((p) => p + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}
