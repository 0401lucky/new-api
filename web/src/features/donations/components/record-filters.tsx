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
import type { Table } from '@tanstack/react-table'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'

import { intakeLabel, rewardLabel } from '../lib/labels'
import type { DonationRecord, DonationRecordFilters } from '../types'

const optionalID = z
  .string()
  .refine(
    (value) =>
      value === '' ||
      (/^[1-9][0-9]*$/.test(value) && Number.isSafeInteger(Number(value))),
    'IDs must be positive integers.'
  )
const schema = z
  .object({
    user_id: optionalID,
    campaign_id: optionalID,
    group_id: optionalID,
    credential_id: optionalID,
    item_id: z
      .string()
      .refine(
        (value) => value === '' || z.uuid().safeParse(value).success,
        'Enter a valid record ID.'
      ),
    state: z.string(),
    reward_state: z.string(),
    from_at_ms: z.number().optional(),
    to_at_ms: z.number().optional(),
  })
  .refine(
    (value) =>
      !value.from_at_ms ||
      !value.to_at_ms ||
      value.from_at_ms <= value.to_at_ms,
    { message: 'End time must be after start time', path: ['to_at_ms'] }
  )

const defaults: z.infer<typeof schema> = {
  user_id: '',
  campaign_id: '',
  group_id: '',
  credential_id: '',
  item_id: '',
  state: 'all',
  reward_state: 'all',
}

export function DonationRecordFilterBar(props: {
  table: Table<DonationRecord>
  loading: boolean
  onChange: (filters: Omit<DonationRecordFilters, 'p' | 'page_size'>) => void
}) {
  const { t } = useTranslation()
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: defaults,
  })
  const values = form.watch()
  const submit = form.handleSubmit((value) =>
    props.onChange({
      user_id: value.user_id ? Number(value.user_id) : undefined,
      campaign_id: value.campaign_id ? Number(value.campaign_id) : undefined,
      group_id: value.group_id ? Number(value.group_id) : undefined,
      credential_id: value.credential_id
        ? Number(value.credential_id)
        : undefined,
      item_id: value.item_id || undefined,
      state: value.state === 'all' ? undefined : value.state,
      reward_state:
        value.reward_state === 'all' ? undefined : value.reward_state,
      from_at_ms: value.from_at_ms,
      to_at_ms: value.to_at_ms,
    })
  )
  const numericFields = [
    { name: 'user_id', label: t('User ID') },
    { name: 'campaign_id', label: t('Campaign ID') },
    { name: 'group_id', label: t('Group ID') },
  ] as const
  const primary = (
    <>
      {numericFields.map((entry) => (
        <LogsFilterField key={entry.name}>
          <FormField
            control={form.control}
            name={entry.name}
            render={({ field }) => (
              <FormItem>
                <FormLabel className='sr-only'>{entry.label}</FormLabel>
                <FormControl>
                  <LogsFilterInput
                    {...field}
                    placeholder={entry.label}
                    inputMode='numeric'
                    maxLength={16}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </LogsFilterField>
      ))}
      <LogsFilterField>
        <FormField
          control={form.control}
          name='state'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='sr-only'>{t('Key result')}</FormLabel>
              <FormControl>
                <Combobox
                  value={field.value}
                  onValueChange={field.onChange}
                  options={[
                    { value: 'all', label: t('All results') },
                    ...[
                      'pending_review',
                      'rejected',
                      'unconfirmed',
                      'queued',
                      'validating',
                      'committing',
                      'accepted',
                      'retry_pending',
                      'invalid',
                      'existing',
                      'duplicate',
                    ].map((value) => ({ value, label: intakeLabel(value, t) })),
                  ]}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </LogsFilterField>
      <LogsFilterField>
        <FormField
          control={form.control}
          name='reward_state'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='sr-only'>{t('Reward')}</FormLabel>
              <FormControl>
                <Combobox
                  value={field.value}
                  onValueChange={field.onChange}
                  options={[
                    { value: 'all', label: t('All rewards') },
                    ...['none', 'pending', 'paused', 'rewarded'].map(
                      (value) => ({ value, label: rewardLabel(value, t) })
                    ),
                  ]}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </LogsFilterField>
    </>
  )
  const dateFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={values.from_at_ms ? new Date(values.from_at_ms) : undefined}
        end={values.to_at_ms ? new Date(values.to_at_ms) : undefined}
        onChange={({ start, end }) => {
          form.setValue('from_at_ms', start?.getTime())
          form.setValue('to_at_ms', end?.getTime())
        }}
      />
      {form.formState.errors.to_at_ms?.message && (
        <p role='alert' className='text-destructive text-sm'>
          {t(form.formState.errors.to_at_ms.message)}
        </p>
      )}
    </LogsFilterField>
  )
  const advanced = (
    <>
      {dateFilter}
      <LogsFilterField>
        <FormField
          control={form.control}
          name='item_id'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='sr-only'>{t('Record ID')}</FormLabel>
              <FormControl>
                <LogsFilterInput
                  {...field}
                  placeholder={t('Record ID')}
                  maxLength={36}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </LogsFilterField>
      <LogsFilterField>
        <FormField
          control={form.control}
          name='credential_id'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='sr-only'>{t('Credential ID')}</FormLabel>
              <FormControl>
                <LogsFilterInput
                  {...field}
                  placeholder={t('Credential ID')}
                  inputMode='numeric'
                  maxLength={16}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </LogsFilterField>
    </>
  )
  const active = Boolean(
    values.user_id ||
    values.campaign_id ||
    values.group_id ||
    values.credential_id ||
    values.item_id ||
    values.from_at_ms ||
    values.to_at_ms ||
    values.state !== 'all' ||
    values.reward_state !== 'all'
  )
  return (
    <Form {...form}>
      <form noValidate onSubmit={(event) => void submit(event)}>
        <LogsFilterToolbar
          table={props.table}
          primaryFilters={primary}
          advancedFilters={advanced}
          hasActiveFilters={active}
          searchLoading={props.loading}
          onSearch={() => void submit()}
          onReset={() => {
            form.reset(defaults)
            props.onChange({})
          }}
        />
      </form>
    </Form>
  )
}
