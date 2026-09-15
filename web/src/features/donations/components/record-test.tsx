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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestampToDate } from '@/lib/format'

import { useDonationTest } from '../hooks/use-donation-test'
import { testStateLabel, testReasonLabel } from '../lib/labels'
import {
  DONATION_REVIEW_LIMIT_DEFAULTS,
  donationTestSchema,
} from '../lib/schema'
import type {
  DonationReviewContext,
  DonationSession,
  DonationTestMetadata,
  DonationTestResult,
  DonationTestState,
} from '../types'

const TEST_STATE_VARIANT: Record<
  DonationTestState,
  'success' | 'danger' | 'warning'
> = {
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'warning',
  interrupted: 'warning',
  running: 'warning',
}

export function DonationTestSummary(props: { test: DonationTestMetadata }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-col gap-1 text-sm'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge
          label={testStateLabel(props.test.state, t)}
          variant={TEST_STATE_VARIANT[props.test.state]}
          copyable={false}
        />
        <span className='font-mono text-xs'>{props.test.model}</span>
      </div>
      <span className='text-muted-foreground text-xs'>
        {t('Started at')}{' '}
        {formatTimestampToDate(props.test.started_at_ms, 'milliseconds')}
        {props.test.finished_at_ms
          ? ` · ${t('Finished at')} ${formatTimestampToDate(props.test.finished_at_ms, 'milliseconds')}`
          : ''}
      </span>
      {props.test.reason_code && (
        <span className='text-muted-foreground text-xs'>
          {testReasonLabel(props.test.reason_code, t)}
        </span>
      )}
    </div>
  )
}

export function DonationRecordTest(props: {
  session: DonationSession
  itemId: string
  context: DonationReviewContext
  disabled?: boolean
  onResult: (test: DonationTestResult) => void
  onRunningChange: (running: boolean) => void
  onFinished: () => void
}) {
  const { t } = useTranslation()
  const limits = props.context.review_limits ?? DONATION_REVIEW_LIMIT_DEFAULTS
  const { run, start, stop } = useDonationTest(props.session, props.context)
  const form = useForm({
    resolver: zodResolver(donationTestSchema(limits)),
    defaultValues: {
      model: props.context.test_models[0] ?? '',
      prompt: '',
      system_prompt: '',
      max_output_tokens: limits.default_output_tokens,
      stream: true,
    },
  })
  const onResult = props.onResult
  useEffect(() => {
    if (run.result) onResult(run.result)
  }, [run.result, onResult])
  const onRunningChange = props.onRunningChange
  const onFinished = props.onFinished
  useEffect(() => {
    onRunningChange(run.running)
    if (!run.running && (run.result || run.error)) onFinished()
  }, [run.running, run.result, run.error, onRunningChange, onFinished])
  const submit = form.handleSubmit((values) => {
    if (!props.context.can_test || props.disabled) return
    start(props.itemId, {
      expected_item_revision: props.context.item_revision,
      review_target_revision: props.context.review_target_revision,
      model: values.model,
      prompt: values.prompt,
      system_prompt: values.system_prompt || undefined,
      max_output_tokens: values.max_output_tokens,
      stream: values.stream,
    })
  })
  return (
    <section
      aria-label={t('Model test')}
      className='flex flex-col gap-3 rounded-lg border p-3'
    >
      <h3 className='font-medium'>{t('Model test')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'The call uses only the key of this record and the group it was submitted to. Successful calls do not approve the record automatically.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'This call uses upstream quota without charging a site wallet. Stopping it cannot undo upstream usage.'
        )}
      </p>
      {props.context.test_models.length === 0 ? (
        <Alert>
          <AlertDescription>
            {t(
              'This group has no compatible text model, so the record cannot be tested. It can still be reviewed manually.'
            )}
          </AlertDescription>
        </Alert>
      ) : (
        <Form {...form}>
          <form noValidate onSubmit={(event) => void submit(event)}>
            <FieldGroup>
              <FormField
                control={form.control}
                name='model'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Text model')}</FormLabel>
                    <FormControl>
                      <Combobox
                        {...field}
                        value={field.value || null}
                        onValueChange={field.onChange}
                        options={props.context.test_models.map((model) => ({
                          value: model,
                          label: model,
                        }))}
                        placeholder={t('Choose a text model.')}
                        disabled={run.running}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='prompt'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('User prompt')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={3}
                        maxLength={limits.max_prompt_bytes}
                        disabled={run.running}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Prompt and system prompt may contain at most {{bytes}} bytes together.',
                        { bytes: limits.max_prompt_bytes }
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='system_prompt'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('System prompt (optional)')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={2}
                        maxLength={limits.max_prompt_bytes}
                        disabled={run.running}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='max_output_tokens'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Output token limit')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        max={limits.max_output_tokens}
                        value={field.value}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? 0
                              : Number(event.target.value)
                          )
                        }
                        disabled={run.running}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Between 1 and {{max}} tokens.', {
                        max: limits.max_output_tokens,
                      })}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='stream'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-4'>
                    <FormLabel>{t('Stream the response')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={run.running}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </FieldGroup>
            <div className='mt-3 flex flex-wrap items-center gap-2'>
              <Button
                type='submit'
                disabled={
                  run.running || !props.context.can_test || props.disabled
                }
              >
                {run.running && <Spinner data-icon='inline-start' />}
                {t('Run test')}
              </Button>
              {run.running && (
                <Button type='button' variant='outline' onClick={stop}>
                  {t('Stop test')}
                </Button>
              )}
            </div>
          </form>
        </Form>
      )}
      <section aria-label={t('Test response')} className='flex flex-col gap-2'>
        <h4 className='text-sm font-medium'>{t('Test response')}</h4>
        {run.error && (
          <Alert variant={run.result ? 'default' : 'destructive'}>
            <AlertDescription>{t(run.error)}</AlertDescription>
          </Alert>
        )}
        {run.result && <DonationTestSummary test={run.result} />}
        {run.text && (
          <pre className='bg-muted max-h-64 overflow-auto rounded-md p-2 text-xs break-words whitespace-pre-wrap'>
            {run.text}
          </pre>
        )}
      </section>
    </section>
  )
}
