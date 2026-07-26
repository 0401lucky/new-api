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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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

import { listAdminUserSubscriptions } from '../../api'
import { formatTimestamp } from '../../lib'
import {
  getEffectiveSubscriptionStatus,
  getSubscriptionUsagePercent,
} from '../../lib/subscription-status'
import { useSubscriptions } from '../subscriptions-provider'

const PAGE_SIZE = 20

export function PlanSubscribersDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useSubscriptions()
  const isOpen = open === 'plan-subscribers'
  const plan = currentRow?.plan
  const planLabel = plan?.title || (plan?.id ? `#${plan.id}` : '-')
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('active')

  useEffect(() => {
    if (isOpen) {
      setPage(1)
      setStatus('active')
    }
  }, [isOpen, plan?.id])

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-plan-subscribers',
      plan?.id,
      page,
      PAGE_SIZE,
      status,
      isOpen,
    ],
    enabled: isOpen && !!plan?.id,
    queryFn: async () => {
      const res = await listAdminUserSubscriptions({
        p: page,
        page_size: PAGE_SIZE,
        plan_id: plan?.id,
        status: status === 'all' ? undefined : status,
      })
      return res.data
    },
    placeholderData: (prev) => prev,
  })

  const items = data?.items || []
  const total = data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <Dialog open={isOpen} onOpenChange={(v) => !v && setOpen(null)}>
      <DialogContent className='flex max-h-[85vh] max-w-4xl flex-col gap-3 overflow-hidden sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{t('Plan subscribers')}</DialogTitle>
          <DialogDescription>
            {t('Subscribers of plan {{plan}}', { plan: planLabel })}
          </DialogDescription>
        </DialogHeader>

        <div className='flex items-center gap-2'>
          <Select
            items={[
              { value: 'all', label: t('All statuses') },
              { value: 'active', label: t('Active') },
              { value: 'expired', label: t('Expired') },
              { value: 'cancelled', label: t('Invalidated') },
            ]}
            value={status}
            onValueChange={(v) => {
              if (v) {
                setStatus(v)
                setPage(1)
              }
            }}
          >
            <SelectTrigger className='w-[160px]'>
              <SelectValue />
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
          <span className='text-muted-foreground text-xs'>
            {t('Total {{count}} records', { count: total })}
          </span>
        </div>

        <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
          <table className='w-full min-w-[700px] text-sm'>
            <thead className='bg-muted/50 sticky top-0'>
              <tr className='border-b text-left'>
                <th className='px-3 py-2 font-medium'>{t('ID')}</th>
                <th className='px-3 py-2 font-medium'>{t('User')}</th>
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
                    colSpan={6}
                    className='text-muted-foreground px-3 py-8 text-center'
                  >
                    {t('Loading...')}
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td
                    colSpan={6}
                    className='text-muted-foreground px-3 py-8 text-center'
                  >
                    {t('No subscription records')}
                  </td>
                </tr>
              ) : (
                items.map((item) => {
                  const sub = item.subscription
                  const effective = getEffectiveSubscriptionStatus(sub)
                  const totalAmt = Number(sub.amount_total || 0)
                  const used = Number(sub.amount_used || 0)
                  const pct = getSubscriptionUsagePercent(used, totalAmt)
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
                        {effective === 'active' ? (
                          <StatusBadge
                            label={t('Active')}
                            variant='success'
                            copyable={false}
                          />
                        ) : effective === 'cancelled' ? (
                          <StatusBadge
                            label={t('Invalidated')}
                            variant='neutral'
                            copyable={false}
                          />
                        ) : (
                          <StatusBadge
                            label={t('Expired')}
                            variant='neutral'
                            copyable={false}
                          />
                        )}
                      </td>
                      <td className='px-3 py-2'>
                        {totalAmt <= 0 ? (
                          <div className='text-sm'>
                            <div>{t('Unlimited')}</div>
                            <div className='text-muted-foreground text-xs'>
                              {t('Used')}: {formatQuota(used)}
                            </div>
                          </div>
                        ) : (
                          <div className='min-w-[120px] space-y-1'>
                            <div className='flex justify-between text-xs'>
                              <span>
                                {formatQuota(used)} / {formatQuota(totalAmt)}
                              </span>
                              <span className='text-muted-foreground'>
                                {(pct ?? 0).toFixed(0)}%
                              </span>
                            </div>
                            <Progress value={pct ?? 0} className='h-1.5' />
                          </div>
                        )}
                      </td>
                      <td className='px-3 py-2 text-xs'>
                        <div>
                          {t('Start')}: {formatTimestamp(sub.start_time)}
                        </div>
                        <div>
                          {t('End')}: {formatTimestamp(sub.end_time)}
                        </div>
                      </td>
                      <td className='px-3 py-2 text-xs'>{sub.source || '-'}</td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>

        <div className='flex items-center justify-end gap-2'>
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
      </DialogContent>
    </Dialog>
  )
}
