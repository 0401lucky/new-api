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
import { useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
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
import { manualBanUser } from '../api'
import {
  MANUAL_BAN_FORM_DEFAULT_VALUES,
  getManualBanFormSchema,
  type ManualBanFormValues,
} from '../lib'
import { useBlackroom } from './blackroom-provider'

export function ManualBanDialog() {
  const { t } = useTranslation()
  const { open, setOpen, triggerRefresh } = useBlackroom()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isOpen = open === 'manual-ban'

  const form = useForm<ManualBanFormValues>({
    resolver: zodResolver(
      getManualBanFormSchema(t)
    ) as Resolver<ManualBanFormValues>,
    defaultValues: MANUAL_BAN_FORM_DEFAULT_VALUES,
  })

  const permanent = form.watch('permanent')

  const onSubmit = async (values: ManualBanFormValues) => {
    setIsSubmitting(true)
    try {
      const result = await manualBanUser({
        ...values,
        duration_hours: values.permanent ? 0 : values.duration_hours,
      })
      if (result.success) {
        toast.success(t('User banned successfully'))
        setOpen(null)
        form.reset(MANUAL_BAN_FORM_DEFAULT_VALUES)
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to ban user'))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(value) => {
        if (!value) {
          setOpen(null)
          form.reset(MANUAL_BAN_FORM_DEFAULT_VALUES)
        }
      }}
    >
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Manual ban')}</DialogTitle>
          <DialogDescription>
            {t('Add a user to the blackroom immediately.')}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id='manual-ban-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='user_id'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('User ID')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min='1'
                      placeholder={t('Enter user ID')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='permanent'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='space-y-1'>
                    <FormLabel>{t('Permanent ban')}</FormLabel>
                    <FormDescription>
                      {t('Keep this user banned until manually released.')}
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
              name='duration_hours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Duration hours')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      min='0'
                      disabled={permanent}
                      placeholder={t('Enter duration in hours')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Ignored when permanent ban is enabled.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='reason'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Reason')}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={3}
                      placeholder={t('Enter ban reason')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
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
          <Button form='manual-ban-form' type='submit' disabled={isSubmitting}>
            {isSubmitting ? t('Saving...') : t('Ban user')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
