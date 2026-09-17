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
import type { TFunction } from 'i18next'
import { ChevronDown } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

import { getBlackroomEvents } from '../api'
import {
  BLACKROOM_SOURCES,
  getBlackroomBanEventConfig,
  normalizeBlackroomSource,
} from '../constants'
import { formatBlackroomTimeValue } from '../lib/blackroom-time'
import type { BlackroomBanEvent } from '../types'
import { useBlackroom } from './blackroom-provider'

const SHADOW_MATCH_EVENT_TYPE = 'shadow_match'

/** 事件里的 evidence / ip_list 都是 JSON 字符串，格式化失败时原样展示。 */
function formatBlackroomJSON(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return trimmed
  }
}

function formatEventHours(seconds: number, t: TFunction): string {
  const hours = seconds / 3600
  return Number.isInteger(hours)
    ? t('{{hours}} hours', { hours })
    : t('{{hours}} hours', { hours: hours.toFixed(1) })
}

function EventEvidence(props: { value: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const formatted = useMemo(
    () => formatBlackroomJSON(props.value),
    [props.value]
  )

  if (!formatted) return null

  return (
    <Collapsible open={open} onOpenChange={setOpen} className='mt-2'>
      <CollapsibleTrigger
        render={
          <Button
            variant='ghost'
            size='sm'
            type='button'
            className='text-muted-foreground hover:text-foreground h-7 gap-1 px-2 text-xs'
          />
        }
      >
        {t('Evidence')}
        <ChevronDown
          className={cn('size-3.5 transition-transform', open && 'rotate-180')}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className='bg-muted mt-1 max-h-64 overflow-auto rounded-md p-2 font-mono text-xs whitespace-pre-wrap'>
          {formatted}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  )
}

function EventTimelineItem(props: { event: BlackroomBanEvent }) {
  const { t } = useTranslation()
  const event = props.event
  const typeConfig = getBlackroomBanEventConfig(event.event_type)
  const sourceConfig = BLACKROOM_SOURCES[
    normalizeBlackroomSource(event.source)
  ] ?? {
    labelKey: event.source || '-',
    variant: 'neutral' as const,
  }
  const isShadowMatch = event.event_type === SHADOW_MATCH_EVENT_TYPE
  const operator =
    event.actor_user_id > 0
      ? t('User {{id}}', { id: event.actor_user_id })
      : t('System')

  return (
    <li className='relative ps-5'>
      <span
        aria-hidden='true'
        className='bg-border absolute start-0 top-2 size-2 rounded-full'
      />
      <div className='rounded-lg border p-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <StatusBadge
            label={t(typeConfig.labelKey)}
            variant={typeConfig.variant}
            copyable={false}
          />
          <StatusBadge
            label={t(sourceConfig.labelKey)}
            variant={sourceConfig.variant}
            copyable={false}
          />
          <span className='text-muted-foreground ms-auto font-mono text-xs'>
            {formatBlackroomTimeValue(event.created_at)}
          </span>
        </div>

        <p className='mt-2 text-sm break-words'>
          {event.reason || t('No reason recorded')}
        </p>

        <div className='text-muted-foreground mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs'>
          {!isShadowMatch && (
            <span>
              {t('Ban duration:')}{' '}
              {event.banned_until === 0
                ? t('Permanent')
                : formatEventHours(event.ban_duration_seconds, t)}
            </span>
          )}
          {!isShadowMatch && event.banned_until > 0 && (
            <span>
              {t('Banned until:')}{' '}
              {formatBlackroomTimeValue(event.banned_until)}
            </span>
          )}
          {event.ip_count > 0 && (
            <span>
              {t('IP Count')}: {event.ip_count}
            </span>
          )}
          <span>
            {t('Operator:')} {operator}
          </span>
        </div>

        <EventEvidence value={event.evidence} />
      </div>
    </li>
  )
}

/**
 * 某个用户的封禁事件时间线。事件只追加不可变，因此这里能回答
 * 「第一次为什么封、第二次为什么延长」。
 */
export function BlackroomEventsDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useBlackroom()
  const isOpen = open === 'events'
  const userId = currentRow?.user_id ?? 0

  const { data, isFetching } = useQuery({
    queryKey: ['blackroom-events', userId],
    queryFn: () => getBlackroomEvents({ user_id: userId }),
    enabled: isOpen && userId > 0,
  })

  const events = data?.data ?? []
  const username = currentRow?.username

  let body: ReactNode = null
  if (isFetching && events.length === 0) {
    body = <LoadingState />
  } else if (events.length === 0) {
    body = <EmptyState title={t('No ban events recorded')} />
  } else {
    body = (
      <ol className='before:bg-border relative space-y-3 before:absolute before:inset-y-2 before:start-1 before:w-px'>
        {events.map((event) => (
          <EventTimelineItem key={event.id} event={event} />
        ))}
      </ol>
    )
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(value) => !value && setOpen(null)}
      title={t('Ban events')}
      description={
        username
          ? t('{{username}} (ID: {{id}})', { username, id: userId })
          : t('User {{id}}', { id: userId })
      }
      contentClassName='sm:max-w-3xl'
    >
      {body}
    </Dialog>
  )
}
