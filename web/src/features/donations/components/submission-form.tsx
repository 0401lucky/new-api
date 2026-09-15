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
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { formatQuota } from '@/lib/format'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { donationApi, donationQueryKey } from '../api'
import { useDonationSubmission } from '../hooks/use-donation-submission'
import { reasonLabel } from '../lib/labels'
import type { DonationBatchDetail, DonationSession } from '../types'
import { DonationRiskNotice } from './risk-notice'

export function DonationSubmissionForm(props: {
  session: DonationSession
  resume: DonationBatchDetail | null
  observedBatch?: DonationBatchDetail | null
  onResult: (batch: DonationBatchDetail) => void
  onNew: () => void
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const campaigns = useQuery({
    queryKey: donationQueryKey(props.session, 'campaigns'),
    queryFn: ({ signal }) => donationApi.campaigns(props.session, signal),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const action = useDonationSubmission(props)
  const selectedId = action.form.watch('campaign_id')
  const selected = campaigns.data?.find(
    (campaign) => campaign.id === selectedId
  )
  const options = (campaigns.data ?? []).map((campaign) => ({
    value: String(campaign.id),
    label: campaign.name,
    disabled: !campaign.available,
    description: campaign.available
      ? t('Permanent reward per key: {{quota}}', {
          quota: formatQuota(campaign.reward_quota),
        })
      : reasonLabel(campaign.unavailable_reason, t),
  }))
  if (
    props.resume &&
    !options.some(
      (option) => option.value === String(props.resume?.campaign_id)
    )
  ) {
    options.push({
      value: String(props.resume.campaign_id),
      label: props.resume.campaign_name,
      disabled: true,
      description: t('Original campaign'),
    })
  }
  const isResuming = props.resume !== null || action.unconfirmed
  const canSubmit =
    isResuming || Boolean(selected?.available && !campaigns.isError)
  const reward = props.resume?.reward_quota ?? selected?.reward_quota
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {isResuming ? t('Resume submission') : t('Donate API keys')}
        </CardTitle>
        <CardDescription>
          {t(
            'Support the community with usable keys. Each newly accepted key earns permanent quota.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {campaigns.isPending && !isResuming ? <LoadingState size='sm' /> : null}
        {campaigns.isError && !isResuming ? (
          <ErrorState
            title={t('Unable to load campaigns')}
            description={t(campaigns.error.message)}
            onRetry={() => void campaigns.refetch()}
            className='min-h-0 py-6'
          />
        ) : null}
        {!campaigns.isPending &&
        !campaigns.isError &&
        campaigns.data?.length === 0 &&
        !isResuming ? (
          <EmptyState
            title={t('No donation campaigns')}
            description={t(
              'There are no campaigns available yet. Please check back later.'
            )}
            className='min-h-0 py-6'
          />
        ) : null}
        <Form {...action.form}>
          <form
            onSubmit={(event) => {
              if (!canSubmit) {
                event.preventDefault()
                return
              }
              void action.submit(event)
            }}
            className='flex flex-col gap-5'
            autoComplete='off'
          >
            <FieldGroup>
              <FormField
                control={action.form.control}
                name='campaign_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Donation campaign')}</FormLabel>
                    <FormControl>
                      <Combobox
                        {...field}
                        value={field.value ? String(field.value) : null}
                        onValueChange={(value) => field.onChange(Number(value))}
                        options={options}
                        placeholder={t('Choose a donation campaign.')}
                        disabled={
                          action.mutation.isPending ||
                          isResuming ||
                          campaigns.isPending ||
                          campaigns.isError
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {selected?.description && (
                        <span className='block'>{selected.description}</span>
                      )}
                      {selected?.validation_mode === 'manual_review' && (
                        <span className='block'>
                          {t(
                            'An administrator reviews each key manually. The reward is credited only after the review passes and the key is received.'
                          )}
                        </span>
                      )}
                      {reward !== undefined && (
                        <span className='block'>
                          {t('Permanent reward per key: {{quota}}', {
                            quota: formatQuota(reward),
                          })}
                        </span>
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {isResuming && (
                <Alert>
                  <AlertDescription>
                    {t(
                      'Receipt is not confirmed. Keep or re-enter the original keys in the same line positions and retry this submission.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              <FormField
                control={action.form.control}
                name='keys_text'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('API keys')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={7}
                        className='min-h-40 font-mono'
                        spellCheck={false}
                        autoComplete='off'
                        autoCorrect='off'
                        autoCapitalize='none'
                        disabled={
                          action.mutation.isPending || action.unconfirmed
                        }
                        placeholder={t('Paste one API key per line')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'One key per line. Up to 100 keys per submission. Blank lines are ignored.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </FieldGroup>
            <DonationRiskNotice />
            {action.form.formState.errors.root?.message && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {t(action.form.formState.errors.root.message)}
                </AlertDescription>
              </Alert>
            )}
            <div className='flex flex-wrap items-center gap-2'>
              <Button
                type='submit'
                disabled={action.mutation.isPending || !canSubmit}
              >
                {action.mutation.isPending && (
                  <Spinner data-icon='inline-start' />
                )}
                {isResuming ? t('Retry original submission') : t('Donate keys')}
              </Button>
              {isResuming && (
                <Button
                  type='button'
                  variant='outline'
                  disabled={action.mutation.isPending}
                  onClick={() => {
                    action.reset()
                    props.onNew()
                  }}
                >
                  {t('Start a new submission')}
                </Button>
              )}
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Only newly accepted keys receive a reward. Existing and invalid keys do not.'
                )}
              </p>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}
