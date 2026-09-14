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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isCancel } from 'axios'
import { useId } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay } from '@/lib/currency'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { donationApi, donationQueryKey, donationSessionIsCurrent } from '../api'
import { useDonationLifetime } from '../hooks/use-donation-session'
import { reasonLabel } from '../lib/labels'
import {
  donationCampaignSchema,
  type DonationCampaignValues,
} from '../lib/schema'
import type { DonationSession, ManagedCampaign } from '../types'

export function DonationCampaignForm(props: {
  session: DonationSession
  campaign: ManagedCampaign | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  useSystemConfigStore((state) => state.config.currency)
  const queryClient = useQueryClient()
  const formId = useId()
  const form = useForm<DonationCampaignValues>({
    resolver: zodResolver(donationCampaignSchema(props.campaign?.reward_quota)),
    defaultValues: {
      name: props.campaign?.name ?? '',
      description: props.campaign?.description ?? '',
      group_id: props.campaign?.group_id ?? 0,
      reward_amount: props.campaign
        ? String(quotaUnitsToDollars(props.campaign.reward_quota))
        : '',
      enabled: props.campaign?.enabled ?? false,
    },
  })
  const getSignal = useDonationLifetime(props.session, () => {})
  const groups = useQuery({
    queryKey: donationQueryKey(props.session, 'groups'),
    queryFn: ({ signal }) => donationApi.groups(props.session, signal),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const selectedId = form.watch('group_id')
  const enabled = form.watch('enabled')
  const selected = groups.data?.find((group) => group.id === selectedId)
  const selectedAvailable = Boolean(
    selected?.enabled &&
    selected.can_probe &&
    selected.connection_type === 'api_key'
  )
  const closing = Boolean(
    props.campaign && !enabled && selectedId === props.campaign.group_id
  )
  const canSave =
    closing || (!groups.isError && !groups.isPending && selectedAvailable)
  const options = (groups.data ?? []).map((group) => ({
    value: String(group.id),
    label: group.name,
    disabled:
      !group.enabled || !group.can_probe || group.connection_type !== 'api_key',
    description: group.can_probe
      ? group.channel_id
      : reasonLabel(group.unavailable_reason, t),
  }))
  if (
    props.campaign &&
    !options.some((option) => option.value === String(props.campaign?.group_id))
  ) {
    options.push({
      value: String(props.campaign.group_id),
      label: props.campaign.group_name,
      disabled: true,
      description: t('Selected group is unavailable.'),
    })
  }
  const mutation = useMutation({
    mutationFn: () => {
      const values = form.getValues()
      // Preserve exact quota on a rename/close; display rounding must not change it.
      const rewardQuota =
        props.campaign && !form.getFieldState('reward_amount').isDirty
          ? props.campaign.reward_quota
          : parseQuotaFromDollars(Number(values.reward_amount))
      return donationApi.saveCampaign(
        props.session,
        {
          name: values.name,
          description: values.description,
          group_id: values.group_id,
          reward_quota: rewardQuota,
          enabled: values.enabled,
        },
        props.campaign?.id,
        getSignal()
      )
    },
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
    onSuccess: () => {
      if (!donationSessionIsCurrent(props.session)) return
      void queryClient.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'managed-campaigns'),
      })
      void queryClient.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'campaigns'),
      })
      props.onClose()
    },
    onError: (error) => {
      if (!donationSessionIsCurrent(props.session) || isCancel(error)) return
      form.setError('root', { message: error.message })
      handleServerError(error)
    },
  })
  const { meta } = getCurrencyDisplay()
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
      title={
        props.campaign
          ? t('Edit donation campaign')
          : t('Create donation campaign')
      }
      description={t(
        'Choose a real gpt-load group and a fixed permanent reward for each newly accepted key.'
      )}
      footer={
        <>
          <Button
            variant='outline'
            onClick={props.onClose}
            disabled={mutation.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form={formId}
            disabled={!canSave || mutation.isPending}
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={formId}
          noValidate
          onSubmit={form.handleSubmit(() => {
            if (canSave) mutation.mutate()
          })}
        >
          <FieldGroup>
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Campaign name')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={120}
                      disabled={mutation.isPending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Description')}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={3}
                      maxLength={4000}
                      disabled={mutation.isPending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='group_id'
              render={({ field }) => (
                <FormItem>
                  <div className='flex items-center justify-between gap-2'>
                    <FormLabel>{t('Receiving group')}</FormLabel>
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      disabled={groups.isFetching}
                      onClick={() => void groups.refetch()}
                    >
                      {t('Refresh groups')}
                    </Button>
                  </div>
                  <FormControl>
                    <Combobox
                      {...field}
                      options={options}
                      value={field.value ? String(field.value) : null}
                      onValueChange={(value) => field.onChange(Number(value))}
                      placeholder={t('Choose an available group.')}
                      disabled={
                        mutation.isPending || groups.isPending || groups.isError
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The group determines the key type and validation method.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            {groups.isPending && (
              <LoadingState size='sm' inline message={t('Loading groups...')} />
            )}
            {groups.isError && (
              <ErrorState
                title={t('Unable to load groups')}
                description={t(groups.error.message)}
                onRetry={() => void groups.refetch()}
                className='min-h-0 py-3'
              />
            )}
            {!groups.isPending &&
              !groups.isError &&
              groups.data?.length === 0 && (
                <EmptyState
                  title={t('No receiving groups')}
                  description={t(
                    'Create a group in gpt-load, then refresh this list.'
                  )}
                  className='min-h-0 py-3'
                />
              )}
            {selectedId > 0 &&
              !groups.isPending &&
              !groups.isError &&
              !selectedAvailable && (
                <Alert>
                  <AlertDescription>
                    {t(
                      'Selected group is unavailable. Choose another group or close this campaign.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
            <FormField
              control={form.control}
              name='reward_amount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Permanent reward per key')}</FormLabel>
                  <InputGroup>
                    <InputGroupAddon>
                      {meta.kind === 'tokens' ? t('Quota') : meta.symbol}
                    </InputGroupAddon>
                    <FormControl>
                      <InputGroupInput
                        {...field}
                        type='number'
                        step='any'
                        min='0'
                        disabled={mutation.isPending}
                      />
                    </FormControl>
                  </InputGroup>
                  <FormDescription>
                    {Number(field.value) > 0
                      ? formatQuota(parseQuotaFromDollars(Number(field.value)))
                      : t('Rewards are added to the permanent balance.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4'>
                  <div className='flex flex-col gap-1'>
                    <FormLabel>{t('Accept donations')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Closing a campaign stops new submissions. Existing submissions keep their original reward.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={mutation.isPending}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            {form.formState.errors.root?.message && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {t(form.formState.errors.root.message)}
                </AlertDescription>
              </Alert>
            )}
          </FieldGroup>
        </form>
      </Form>
    </Dialog>
  )
}
