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
import { RefreshCw } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

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
import { cn } from '@/lib/utils'

import type {
  UserActivityFilter,
  UserActivitySummary as Summary,
} from '../types'

interface UserActivitySummaryProps {
  summary?: Summary
  activity: UserActivityFilter
  onActivityChange: (activity: UserActivityFilter) => void
  onRefresh: () => void
  isFetching: boolean
}

export function UserActivitySummary(props: UserActivitySummaryProps) {
  const { t, i18n } = useTranslation()
  const id = useId()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const cards = [
    {
      value: 'active',
      label: t('Active users'),
      description: t('Requested within 7 days'),
    },
    {
      value: 'inactive',
      label: t('Inactive users'),
      description: t('Last request 7–30 days ago'),
    },
    {
      value: 'very_inactive',
      label: t('Very inactive users'),
      description: t('No requests for at least 30 days'),
    },
    {
      value: 'never_requested',
      label: t('Never requested'),
      description: t('No requests since registration'),
    },
  ] as const

  return (
    <div className='flex shrink-0 flex-col gap-3'>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
        {cards.map((card) => (
          <Card
            key={card.value}
            data-card-hover='false'
            className={cn(
              'relative min-w-0 gap-2 py-3 outline-offset-2 transition-colors has-focus-visible:outline-ring has-focus-visible:outline-2',
              props.activity === card.value &&
                'bg-primary/5 outline-primary outline-2'
            )}
          >
            <CardHeader>
              <CardDescription
                id={`${id}-${card.value}-label`}
                className='text-xs'
              >
                {card.label}
              </CardDescription>
              <CardTitle
                id={`${id}-${card.value}-value`}
                className='text-2xl leading-none font-semibold break-all tabular-nums'
              >
                {props.summary
                  ? formatNumber(props.summary[card.value], locale)
                  : '—'}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p
                id={`${id}-${card.value}-description`}
                className='text-muted-foreground text-xs'
              >
                {card.description}
              </p>
            </CardContent>
            <Button
              variant='ghost'
              aria-labelledby={`${id}-${card.value}-label ${id}-${card.value}-value ${id}-${card.value}-description`}
              aria-pressed={props.activity === card.value}
              onClick={() =>
                props.onActivityChange(
                  props.activity === card.value ? '' : card.value
                )
              }
              className='hover:bg-primary/5 focus-visible:bg-primary/5 absolute inset-0 h-full w-full rounded-[inherit]! p-0 focus-visible:ring-0'
            />
          </Card>
        ))}
      </div>
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          variant={props.activity === '' ? 'secondary' : 'outline'}
          aria-pressed={props.activity === ''}
          onClick={() => props.onActivityChange('')}
        >
          {t('All users')} ·{' '}
          {props.summary ? formatNumber(props.summary.total, locale) : '—'}
        </Button>
        <Button
          variant={props.activity === 'cleanup' ? 'secondary' : 'outline'}
          aria-pressed={props.activity === 'cleanup'}
          onClick={() => props.onActivityChange('cleanup')}
        >
          {t('Eligible for 30-day cleanup')} ·{' '}
          {props.summary
            ? formatNumber(props.summary.cleanup_candidates, locale)
            : '—'}
        </Button>
        {props.summary?.unknown || props.activity === 'unknown' ? (
          <Button
            variant={props.activity === 'unknown' ? 'secondary' : 'outline'}
            aria-pressed={props.activity === 'unknown'}
            onClick={() => props.onActivityChange('unknown')}
          >
            {t('Incomplete history')} ·{' '}
            {props.summary ? formatNumber(props.summary.unknown, locale) : '—'}
          </Button>
        ) : null}
        <Button
          variant='ghost'
          size='icon'
          aria-label={t('Refresh')}
          disabled={props.isFetching}
          onClick={props.onRefresh}
          className='ml-auto'
        >
          <RefreshCw
            className={cn('size-4', props.isFetching && 'animate-spin')}
            aria-hidden='true'
          />
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Activity includes successful and failed API requests. Signing in does not count.'
        )}{' '}
        {t(
          'Batch cleanup is limited to regular users registered at least 30 days ago with no requests for at least 30 days.'
        )}
      </p>
      {Boolean(props.summary?.unknown) && (
        <p className='text-muted-foreground text-xs' role='note'>
          {t(
            'Historical activity uses retained request logs. Accounts with missing history are listed separately and require 30 days of observation before cleanup.'
          )}
        </p>
      )}
    </div>
  )
}
