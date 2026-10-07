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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
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
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getGroupMultipliers } from './api'
import {
  formatMultiplier,
  multiplierModeLabel,
  type GroupMultiplierStatus,
} from './group-policy'
import { GroupPolicyDialog } from './group-policy-dialog'

export function GroupMultiplierCard(props: {
  status: GroupMultiplierStatus
  onEdit?: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const status = props.status
  const free = status.base_ratio === 0
  const tiers = status.policy.tiers
  const highest = Math.max(1, ...tiers.map((tier) => tier.multiplier))
  return (
    <Card className='min-w-0'>
      <CardHeader>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle className='break-all'>{status.group}</CardTitle>
          <Badge variant='secondary'>
            {multiplierModeLabel(status.policy.mode, t)}
          </Badge>
        </div>
        {status.description && (
          <CardDescription className='break-words'>
            {status.description}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='space-y-1'>
          <p className='text-muted-foreground text-xs'>
            {t('Current group multiplier')}
          </p>
          <p className='text-primary text-3xl font-semibold break-words tabular-nums'>
            {free
              ? t('Free of charge')
              : formatMultiplier(status.effective_ratio, locale)}
          </p>
          {status.policy.mode === 'balance' && !free ? (
            <p className='text-muted-foreground text-sm'>
              {t('Varies by balance, model, and time rules.')}
            </p>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('Base multiplier')}{' '}
              {formatMultiplier(status.base_ratio, locale)} ·{' '}
              {t('Dynamic multiplier')}{' '}
              {formatMultiplier(status.factor, locale)}
            </p>
          )}
        </div>
        {status.policy.mode === 'concurrency' && !free && (
          <div className='overflow-x-auto pb-1'>
            <ol
              aria-label={t('Concurrency tiers')}
              className='flex min-w-full items-end gap-1'
            >
              {tiers.map((tier, index) => {
                const active =
                  status.concurrency != null &&
                  status.concurrency >= tier.minimum &&
                  (index === tiers.length - 1 ||
                    status.concurrency < tiers[index + 1].minimum)
                return (
                  <li
                    key={tier.minimum}
                    aria-current={active ? 'step' : undefined}
                    className='min-w-14 flex-1 text-center text-xs'
                  >
                    <div
                      className={cn(
                        'flex items-center justify-center rounded-t-md px-1 tabular-nums',
                        active
                          ? 'bg-primary text-primary-foreground font-semibold'
                          : 'bg-primary/10 text-foreground'
                      )}
                      style={{
                        height: `${36 + (56 * tier.multiplier) / highest}px`,
                      }}
                    >
                      {formatMultiplier(tier.multiplier, locale)}
                    </div>
                    <p className='text-muted-foreground pt-2'>
                      ≥{formatNumber(tier.minimum, locale)}
                    </p>
                  </li>
                )
              })}
            </ol>
          </div>
        )}
        <div className='space-y-1 text-sm'>
          <p>
            {t('In progress')}:{' '}
            <span className='tabular-nums'>
              {status.concurrency == null
                ? t('Unavailable')
                : formatNumber(status.concurrency, locale)}
            </span>
          </p>
          {status.next_request_ratio != null && (
            <p>
              {t('Next request estimate')}:{' '}
              <strong className='tabular-nums'>
                {free
                  ? t('Free of charge')
                  : formatMultiplier(status.next_request_ratio, locale)}
              </strong>
            </p>
          )}
          {status.next_tier && !free && (
            <p className='text-muted-foreground'>
              {t('Next tier')}: ≥
              {formatNumber(status.next_tier.minimum, locale)} →{' '}
              {formatMultiplier(
                status.base_ratio * status.next_tier.multiplier,
                locale
              )}
            </p>
          )}
        </div>
        {props.onEdit && (
          <Button variant='outline' size='sm' onClick={props.onEdit}>
            {t('Configure multiplier')}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

export function GroupMultiplierPanel(props: { admin?: boolean }) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [editing, setEditing] = useState<GroupMultiplierStatus | null>(null)
  const query = useQuery({
    queryKey: ['group-multipliers', Boolean(props.admin), user?.id],
    queryFn: ({ signal }) => getGroupMultipliers(Boolean(props.admin), signal),
    enabled: Boolean(user),
    refetchInterval: 5000,
  })
  const canEdit = props.admin && (user?.role ?? 0) >= ROLE.SUPER_ADMIN
  return (
    <section className='space-y-4' aria-label={t('Group multipliers')}>
      <div className='space-y-1'>
        <h3 className='text-base font-semibold'>{t('Group multipliers')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Updates every 5 seconds. Estimates may change before admission; accepted requests keep their multiplier.'
          )}
        </p>
      </div>
      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {!query.isPending &&
        !query.isError &&
        (query.data?.length ? (
          <div className='grid min-w-0 gap-4 md:grid-cols-2 xl:grid-cols-3'>
            {query.data.map((status) => (
              <GroupMultiplierCard
                key={status.group}
                status={status}
                onEdit={canEdit ? () => setEditing(status) : undefined}
              />
            ))}
          </div>
        ) : (
          <EmptyState title={t('No available groups')} />
        ))}
      {editing && (
        <GroupPolicyDialog
          key={editing.group}
          status={editing}
          onClose={() => setEditing(null)}
        />
      )}
    </section>
  )
}
