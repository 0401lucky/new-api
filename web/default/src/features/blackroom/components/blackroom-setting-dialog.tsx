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
import { useEffect, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getBlackroomSetting, updateBlackroomSetting } from '../api'
import {
  BLACKROOM_SETTING_FORM_DEFAULT_VALUES,
  getBlackroomSettingFormSchema,
  transformFormValuesToSetting,
  transformSettingToFormDefaults,
  type BlackroomSettingFormValues,
} from '../lib'
import { useBlackroom } from './blackroom-provider'

export function BlackroomSettingDialog() {
  const { t } = useTranslation()
  const { open, setOpen, triggerRefresh } = useBlackroom()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isOpen = open === 'setting'

  const { data, isFetching } = useQuery({
    queryKey: ['blackroom-setting'],
    queryFn: getBlackroomSetting,
    enabled: isOpen,
  })

  const form = useForm<BlackroomSettingFormValues>({
    resolver: zodResolver(
      getBlackroomSettingFormSchema(t)
    ) as Resolver<BlackroomSettingFormValues>,
    defaultValues: BLACKROOM_SETTING_FORM_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (isOpen) {
      form.reset(transformSettingToFormDefaults(data?.data))
    }
  }, [data?.data, form, isOpen])

  const onSubmit = async (values: BlackroomSettingFormValues) => {
    setIsSubmitting(true)
    try {
      const result = await updateBlackroomSetting(
        transformFormValuesToSetting(values)
      )
      if (result.success) {
        toast.success(t('Blackroom settings saved'))
        setOpen(null)
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to save blackroom settings'))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={(value) => !value && setOpen(null)}>
      <DialogContent className='max-h-[85vh] sm:max-w-2xl flex flex-col'>
        <DialogHeader>
          <DialogTitle>{t('Blackroom settings')}</DialogTitle>
          <DialogDescription>
            {t('Configure automatic blackroom scanning and ban defaults.')}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id='blackroom-setting-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='min-h-0 flex-1 space-y-4 overflow-y-auto pr-1'
          >
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='space-y-1'>
                    <FormLabel>{t('Enable blackroom')}</FormLabel>
                    <FormDescription>
                      {t('Automatically block users that match risk rules.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='auto_ban_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='space-y-1'>
                    <FormLabel>{t('Enable auto ban')}</FormLabel>
                    <FormDescription>
                      {t('Run scheduled scans and ban users that match rules.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='lookback_hours'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Lookback hours')}</FormLabel>
                    <FormControl>
                      <Input {...field} type='number' min='1' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='check_interval_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Scan interval minutes')}</FormLabel>
                    <FormControl>
                      <Input {...field} type='number' min='1' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='min_requests'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum requests')}</FormLabel>
                    <FormControl>
                      <Input {...field} type='number' min='0' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='escalation_window_days'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Escalation window days')}</FormLabel>
                    <FormControl>
                      <Input {...field} type='number' min='1' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='escalation_temporary_ban_count'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Temporary bans before permanent')}
                    </FormLabel>
                    <FormControl>
                      <Input {...field} type='number' min='0' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name='rules_text'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Rules JSON')}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={7}
                      className='font-mono text-xs'
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Each rule needs ip_count, duration_hours, and permanent.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='exempt_user_ids_text'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Exempt user IDs')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={3}
                        placeholder={t('Comma or newline separated')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='exempt_groups_text'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Exempt groups')}</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={3}
                        placeholder={t('Comma or newline separated')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </form>
        </Form>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={isSubmitting}
            onClick={() => setOpen(null)}
          >
            {t('Cancel')}
          </Button>
          <Button
            form='blackroom-setting-form'
            type='submit'
            disabled={isSubmitting || isFetching}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
