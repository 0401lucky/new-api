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
import type { ReactNode } from 'react'
import { Radio, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { availabilityColorClass, STATUS_META } from '../status'
import type { ModelHealthOverviewModel } from '../types'
import { formatLatencyMs, formatRate, formatTokens } from '../utils'
import { HealthTimeline } from './health-timeline'

function MetricCell(props: {
  icon: ReactNode
  label: string
  value: string
}) {
  return (
    <div className='bg-muted/50 rounded-lg p-3'>
      <div className='text-muted-foreground flex items-center gap-2'>
        {props.icon}
        <span className='text-[10px] font-semibold tracking-wider uppercase'>
          {props.label}
        </span>
      </div>
      <div className='text-foreground mt-1 font-mono text-lg leading-none font-medium'>
        {props.value}
      </div>
    </div>
  )
}

export function ModelHealthCard(props: {
  model: ModelHealthOverviewModel
  periodLabel: string
}) {
  const { t } = useTranslation()
  const { model } = props
  const meta = STATUS_META[model.status]

  return (
    <div className='bg-card hover:border-foreground/20 flex flex-col gap-4 rounded-xl border p-4 transition-colors sm:p-5'>
      <div className='flex items-center gap-3'>
        <div className='min-w-0 flex-1'>
          <Tooltip>
            <TooltipTrigger className='block w-full truncate text-left font-mono text-base font-semibold tracking-tight'>
              {model.model_name}
            </TooltipTrigger>
            <TooltipContent className='break-all'>
              {model.model_name}
            </TooltipContent>
          </Tooltip>
          <div className='text-muted-foreground mt-0.5 text-xs'>
            {formatTokens(model.success_tokens_24h)} {t('tokens')} ·{' '}
            {t('Past 24 hours')}
          </div>
        </div>
        <Badge
          variant='outline'
          className={cn(
            'shrink-0 px-2 py-0.5 text-[10px] font-semibold whitespace-nowrap sm:text-xs',
            meta.badgeClass
          )}
        >
          <span className={cn('size-1.5 rounded-full', meta.dotClass)} />
          {t(meta.labelKey)}
        </Badge>
      </div>

      <div className='grid grid-cols-2 gap-3'>
        <MetricCell
          icon={<Zap className='h-3.5 w-3.5' />}
          label={t('Avg latency')}
          value={formatLatencyMs(model.avg_latency_ms)}
        />
        <MetricCell
          icon={<Radio className='h-3.5 w-3.5' />}
          label={t('TTFT')}
          value={formatLatencyMs(model.avg_ttft_ms)}
        />
      </div>

      <div className='bg-muted/30 flex items-center justify-between rounded-lg px-3 py-2'>
        <div className='space-y-0.5'>
          <p className='text-muted-foreground text-[10px] font-semibold tracking-wider uppercase'>
            {t('Availability')} ({props.periodLabel})
          </p>
          <p className='text-muted-foreground text-[10px]'>
            {model.availability === null
              ? t('No data')
              : t('{{success}}/{{total}} succeeded', {
                  success: model.availability_success,
                  total: model.availability_total,
                })}
          </p>
        </div>
        <span
          className={cn(
            'font-mono text-sm font-bold',
            availabilityColorClass(model.availability)
          )}
        >
          {model.availability === null ? '—' : formatRate(model.availability)}
        </span>
      </div>

      <div className='border-t pt-3'>
        <HealthTimeline items={model.timeline} />
      </div>
    </div>
  )
}
