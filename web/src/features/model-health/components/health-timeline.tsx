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
import { useTranslation } from 'react-i18next'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { STATUS_META, timelineStatusFromRate } from '../status'
import type { ModelHealthOverviewTimelineItem } from '../types'
import { formatRate, formatTokens, hourLabel } from '../utils'

export function HealthTimeline(props: {
  items: ModelHealthOverviewTimelineItem[]
}) {
  const { t } = useTranslation()

  return (
    <div className='space-y-1.5'>
      <div className='flex h-8 w-full items-stretch gap-[3px]'>
        {props.items.map((item) => {
          const status = timelineStatusFromRate(
            item.total_requests,
            item.success_rate
          )
          const meta = STATUS_META[status]
          const hasData = item.total_requests > 0
          return (
            <Tooltip key={item.hour_start_ts}>
              <TooltipTrigger
                render={
                  <div
                    className={cn(
                      'min-w-0 flex-1 cursor-pointer rounded-[2px] transition-transform duration-150 hover:scale-y-110 hover:opacity-80',
                      meta.barClass
                    )}
                    aria-label={`${hourLabel(item.hour_start_ts)} ${
                      hasData ? formatRate(item.success_rate) : t('No data')
                    }`}
                  />
                }
              />
              <TooltipContent className='p-3 text-xs'>
                <div className='mb-1.5 text-sm font-semibold'>
                  {hourLabel(item.hour_start_ts)}
                </div>
                {hasData ? (
                  <div className='space-y-1'>
                    <div>
                      {t('Success rate')}:{' '}
                      <span className='font-medium'>
                        {formatRate(item.success_rate)}
                      </span>
                    </div>
                    <div>
                      {t('Total requests')}:{' '}
                      <span className='font-medium'>{item.total_requests}</span>
                    </div>
                    <div>
                      {t('Error requests')}:{' '}
                      <span className='font-medium'>{item.error_requests}</span>
                    </div>
                    <div>
                      {t('Total tokens')}:{' '}
                      <span className='font-medium'>
                        {formatTokens(item.success_tokens)}
                      </span>
                    </div>
                  </div>
                ) : (
                  <div className='text-muted-foreground italic'>
                    {t('No data')}
                  </div>
                )}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
      <div className='text-muted-foreground/60 flex justify-between text-[9px] font-medium tracking-widest uppercase'>
        <span>{t('24h ago')}</span>
        <span>{t('Now')}</span>
      </div>
    </div>
  )
}
