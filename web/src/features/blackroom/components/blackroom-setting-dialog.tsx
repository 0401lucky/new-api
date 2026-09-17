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
import { useEffect, useState, type ReactNode } from 'react'
import { useForm, type Control, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { StatusBadge } from '@/components/status-badge'
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
import {
  getBlackroomSetting,
  getBlackroomStatus,
  updateBlackroomSetting,
} from '../api'
import {
  BLACKROOM_SETTING_FORM_DEFAULT_VALUES,
  getBlackroomSettingFormSchema,
  resolveBlackroomGeoReadiness,
  transformFormValuesToSetting,
  transformSettingToFormDefaults,
  type BlackroomSettingFormValues,
} from '../lib'
import { useBlackroom } from './blackroom-provider'

function SettingSection(props: {
  titleKey: string
  descriptionKey?: string
  children: ReactNode
}) {
  const { t } = useTranslation()

  return (
    <section className='space-y-3 border-t pt-4 first:border-t-0 first:pt-0'>
      <div className='space-y-0.5'>
        <h3 className='text-sm font-semibold'>{t(props.titleKey)}</h3>
        {props.descriptionKey != null && (
          <p className='text-muted-foreground text-xs'>
            {t(props.descriptionKey)}
          </p>
        )}
      </div>
      {props.children}
    </section>
  )
}

/** 设置表单里的布尔开关字段，决定 SwitchFormField 的可用 name。 */
type BlackroomSwitchFieldName =
  | 'enabled'
  | 'auto_ban_enabled'
  | 'shadow_mode'
  | 'realtime_enabled'
  | 'geo_enabled'

function SwitchFormField(props: {
  control: Control<BlackroomSettingFormValues>
  name: BlackroomSwitchFieldName
  labelKey: string
  descriptionKey: string
}) {
  const { t } = useTranslation()

  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
          <div className='space-y-1'>
            <FormLabel>{t(props.labelKey)}</FormLabel>
            <FormDescription>{t(props.descriptionKey)}</FormDescription>
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
  )
}

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

  const { data: statusData } = useQuery({
    queryKey: ['blackroom-status'],
    queryFn: getBlackroomStatus,
    enabled: isOpen,
  })

  const status = statusData?.data
  const geoReadiness = resolveBlackroomGeoReadiness(status)

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
            <SettingSection titleKey='Basics'>
              <SwitchFormField
                control={form.control}
                name='enabled'
                labelKey='Enable blackroom'
                descriptionKey='Automatically block users that match risk rules.'
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
              </div>
            </SettingSection>

            <SettingSection titleKey='Automatic banning'>
              <SwitchFormField
                control={form.control}
                name='auto_ban_enabled'
                labelKey='Enable auto ban'
                descriptionKey='Run scheduled scans and ban users that match rules.'
              />
              <SwitchFormField
                control={form.control}
                name='shadow_mode'
                labelKey='Shadow mode'
                descriptionKey='Record rule matches without banning, so you can observe the rules before enabling them.'
              />
              <SwitchFormField
                control={form.control}
                name='realtime_enabled'
                labelKey='Realtime blocking'
                descriptionKey='Evaluate every relay request and ban immediately on a match, without waiting for the next scan.'
              />
              <div className='grid gap-4 sm:grid-cols-2'>
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
            </SettingSection>

            <SettingSection titleKey='Geo blocking'>
              <SwitchFormField
                control={form.control}
                name='geo_enabled'
                labelKey='Enable geo blocking'
                descriptionKey='Ban users whose requests jump across many countries and networks within a short window. Requires MMDB files.'
              />
              {status != null && (
                <div className='flex flex-wrap items-center gap-2 rounded-lg border border-dashed p-3'>
                  <span className='text-muted-foreground text-xs'>
                    {t('MMDB resolver')}
                  </span>
                  <StatusBadge
                    label={
                      geoReadiness.effective
                        ? t('Effective')
                        : t('Not effective')
                    }
                    variant={geoReadiness.effective ? 'success' : 'warning'}
                    copyable={false}
                  />
                  {geoReadiness.reasonKey != null && (
                    <span className='text-muted-foreground text-xs'>
                      {t(geoReadiness.reasonKey)}
                    </span>
                  )}
                  {status.resolver.version !== '' && (
                    <span className='text-muted-foreground font-mono text-xs'>
                      {t('Version:')} {status.resolver.version}
                    </span>
                  )}
                </div>
              )}
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='geo_country_count'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Country count')}</FormLabel>
                      <FormControl>
                        <Input {...field} type='number' min='1' />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='geo_asn_count'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('ASN count')}</FormLabel>
                      <FormControl>
                        <Input {...field} type='number' min='1' />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='geo_min_gap_seconds'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Minimum IP switch gap (seconds)')}</FormLabel>
                      <FormControl>
                        <Input {...field} type='number' min='1' />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='geo_duration_hours'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Geo ban duration hours')}</FormLabel>
                      <FormControl>
                        <Input {...field} type='number' min='1' />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <FormField
                control={form.control}
                name='country_mmdb_path'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Country MMDB path')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('/path/to/GeoLite2-Country.mmdb')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='asn_mmdb_path'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('ASN MMDB path')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('/path/to/GeoLite2-ASN.mmdb')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SettingSection>

            <SettingSection
              titleKey='Exemptions'
              descriptionKey='Exempt users and groups are never auto banned.'
            >
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
            </SettingSection>
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
