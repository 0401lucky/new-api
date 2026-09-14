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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isCancel } from 'axios'
import { useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

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
import { handleServerError } from '@/lib/handle-server-error'

import { donationApi, donationQueryKey, donationSessionIsCurrent } from '../api'
import { useDonationLifetime } from '../hooks/use-donation-session'
import { donationConnectionSchema } from '../lib/schema'
import type { DonationConnection, DonationSession } from '../types'

type ConnectionValues = z.infer<typeof donationConnectionSchema>

export function DonationConnectionForm(props: {
  session: DonationSession
  connection: DonationConnection
  canWrite: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const pending = useRef<ConnectionValues | null>(null)
  const form = useForm<ConnectionValues>({
    resolver: zodResolver(donationConnectionSchema),
    defaultValues: { base_url: props.connection.base_url, token: '' },
  })
  const getSignal = useDonationLifetime(props.session, () => {
    pending.current = null
    form.setValue('token', '')
  })
  const mutation = useMutation({
    mutationFn: () => {
      const values = pending.current
      if (!values) throw new Error('Enter the integration credential.')
      return donationApi.saveConnection(
        props.session,
        {
          base_url: values.base_url,
          ...(values.token ? { token: values.token } : {}),
        },
        getSignal()
      )
    },
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
    onSuccess: (connection) => {
      if (!donationSessionIsCurrent(props.session)) return
      pending.current = null
      form.reset({ base_url: connection.base_url, token: '' })
      queryClient.setQueryData(
        donationQueryKey(props.session, 'connection'),
        connection
      )
      void queryClient.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'groups'),
      })
      void queryClient.invalidateQueries({
        queryKey: donationQueryKey(props.session, 'campaigns'),
      })
    },
    onError: (error) => {
      pending.current = null
      if (!donationSessionIsCurrent(props.session) || isCancel(error)) return
      form.setError('root', { message: error.message })
      handleServerError(error)
    },
  })
  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <CardTitle>{t('gpt-load connection')}</CardTitle>
          <StatusBadge
            label={
              props.connection.configured
                ? t('Configured')
                : t('Not configured')
            }
            variant={props.connection.configured ? 'success' : 'neutral'}
            copyable={false}
          />
        </div>
        <CardDescription>
          {t(
            'Connect the existing gpt-load service to choose donation groups.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            noValidate
            autoComplete='off'
            className='flex flex-col gap-5'
            onSubmit={form.handleSubmit((values) => {
              if (!props.canWrite) return
              if (!props.connection.configured && !values.token) {
                form.setError('token', {
                  message: 'Enter the integration credential.',
                })
                return
              }
              pending.current = values
              form.clearErrors('root')
              mutation.mutate()
            })}
          >
            {props.connection.configured && (
              <dl className='text-sm'>
                <dt className='text-muted-foreground'>{t('Instance ID')}</dt>
                <dd className='font-mono text-xs break-all'>
                  {props.connection.instance_id}
                </dd>
              </dl>
            )}
            <FieldGroup>
              <FormField
                control={form.control}
                name='base_url'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Service URL')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='url'
                        placeholder='https://gpt-load.example.com'
                        disabled={!props.canWrite || mutation.isPending}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Use HTTPS. Loopback HTTP is supported for local development.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {props.canWrite && (
                <FormField
                  control={form.control}
                  name='token'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Integration credential')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='new-password'
                          maxLength={256}
                          disabled={mutation.isPending}
                          placeholder={
                            props.connection.configured
                              ? t('Leave blank to keep the current credential')
                              : undefined
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Saved credentials are never displayed.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </FieldGroup>
            {form.formState.errors.root?.message && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {t(form.formState.errors.root.message)}
                </AlertDescription>
              </Alert>
            )}
            {mutation.isSuccess && (
              <p role='status' className='text-muted-foreground text-sm'>
                {t('Connection saved and verified.')}
              </p>
            )}
            {props.canWrite && (
              <div>
                <Button type='submit' disabled={mutation.isPending}>
                  {mutation.isPending && <Spinner data-icon='inline-start' />}
                  {t('Save connection')}
                </Button>
              </div>
            )}
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}
