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

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
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

import { getBlackroomDistributionReasonLabel } from '../constants'
import { formatBlackroomTimeValue } from '../lib/blackroom-time'
import type {
  BlackroomDistributionItem,
  BlackroomDistributionToken,
  BlackroomPromptSummary,
} from '../types'

function PromptEvidence(props: {
  label: string
  summary: BlackroomPromptSummary
  limited: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='mt-1 text-sm'>
        {props.summary.sample_count === 0 ? (
          t('No text samples')
        ) : (
          <>
            {t('{{unique}} unique / {{samples}} samples', {
              unique: formatNumber(props.summary.unique_count, locale),
              samples: formatNumber(props.summary.sample_count, locale),
            })}
            <span className='text-muted-foreground mt-1 block text-xs'>
              {t('Top prompt share: {{share}}', {
                share: `${formatNumber(props.summary.dominant_share * 100, locale)}%`,
              })}
            </span>
          </>
        )}
        {props.limited && (
          <span className='text-muted-foreground block text-xs'>
            {t('Partial observation')}
          </span>
        )}
      </dd>
    </div>
  )
}

function TokenEvidence(props: { token: BlackroomDistributionToken }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const token = props.token
  const metrics = [
    { label: t('Requests'), value: formatNumber(token.request_count, locale) },
    {
      label: t('Default Go client'),
      value: `${formatNumber(token.default_go_count, locale)} / ${formatNumber(token.request_count, locale)}`,
    },
    {
      label: t('Hosting ASN with default Go client'),
      value: `${formatNumber(token.hosting_go_count, locale)} / ${formatNumber(token.request_count, locale)}`,
    },
    {
      label: t('Resolved geo requests'),
      value: formatNumber(token.geo_sample_count, locale),
    },
    { label: t('Network prefixes'), value: token.subnets.join(', ') || '-' },
    { label: t('Countries'), value: token.countries.join(', ') || '-' },
    {
      label: t('ASNs'),
      value: token.asns.map((asn) => `AS${asn}`).join(', ') || '-',
    },
    {
      label: t('First Seen'),
      value: formatBlackroomTimeValue(token.first_seen_at),
    },
    {
      label: t('Last Seen'),
      value: formatBlackroomTimeValue(token.last_seen_at),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex flex-wrap items-center justify-between gap-2'>
          <span className='min-w-0 break-all'>
            {token.token_name || t('Key {{id}}', { id: token.token_id })}
          </span>
          <StatusBadge
            label={`${formatNumber(token.score, locale)} / 100`}
            variant={token.score > 0 ? 'warning' : 'neutral'}
            copyable={false}
          />
        </CardTitle>
        <CardDescription>
          {t('Key {{id}}', { id: token.token_id })}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        {(token.prompt_limited || token.network_limited) && (
          <Alert>
            <AlertDescription>
              {t(
                'Observation limits reached. Incomplete prompt or network signals are excluded from the score.'
              )}
            </AlertDescription>
          </Alert>
        )}
        <dl className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
          <PromptEvidence
            label={t('System prompts')}
            summary={token.system_prompt}
            limited={token.prompt_limited}
          />
          <PromptEvidence
            label={t('First user prompts')}
            summary={token.first_user_prompt}
            limited={token.prompt_limited}
          />
          {metrics.map((metric) => (
            <div key={metric.label} className='min-w-0'>
              <dt className='text-muted-foreground text-xs'>{metric.label}</dt>
              <dd className='mt-1 text-sm break-all'>{metric.value}</dd>
            </div>
          ))}
        </dl>
        <div className='flex flex-wrap gap-1'>
          {token.reasons.length === 0 ? (
            <span className='text-muted-foreground text-sm'>
              {t('No review signals')}
            </span>
          ) : (
            token.reasons.map((reason) => (
              <StatusBadge
                key={reason}
                label={getBlackroomDistributionReasonLabel(reason, t)}
                variant='neutral'
                copyable={false}
              />
            ))
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export function BlackroomDistributionDialog(props: {
  item: BlackroomDistributionItem | null
  onClose: () => void
  onSelectUser: (userId: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Dialog
      open={props.item !== null}
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={t('Observation details')}
      description={
        props.item?.username || t('User {{id}}', { id: props.item?.user_id })
      }
      contentHeight='min(65vh, 600px)'
      footer={
        props.item && (
          <Button
            variant='outline'
            onClick={() => {
              if (!props.item) return
              props.onSelectUser(props.item.user_id)
              props.onClose()
            }}
          >
            {t('View ban records')}
          </Button>
        )
      }
    >
      <div className='flex flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Prompt metrics cover the latest minute. Network and client metrics cover the latest 10 minutes.'
          )}
        </p>
        {props.item?.tokens.map((token) => (
          <TokenEvidence key={token.token_id} token={token} />
        ))}
      </div>
    </Dialog>
  )
}
