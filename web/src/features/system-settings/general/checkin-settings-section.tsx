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
import i18next from 'i18next'
import { Clock } from 'lucide-react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { useStatus } from '@/hooks/use-status'

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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { updateCheckinSetting } from '../api'
import { minutesToTime, timeToMinutes } from '../lib/checkin-time'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const DEFAULT_QUOTA_PER_USD = 500000
const MAX_QUOTA = 2147483647

function normalizeQuotaPerUnit(value: number | undefined): number {
  return value && value > 0 ? value : DEFAULT_QUOTA_PER_USD
}

function quotaToUsdAmount(quota: number, quotaPerUnit: number): number {
  return Number((quota / quotaPerUnit).toFixed(6))
}

function usdAmountToQuota(amount: number, quotaPerUnit: number): number {
  if (!Number.isFinite(amount) || amount <= 0) return 0
  return Math.round(amount * quotaPerUnit)
}

const createSchema = (t: (key: string) => string) =>
  z
    .object({
      enabled: z.boolean(),
      rewardType: z.enum(['permanent', 'temporary']),
      availableFrom: z
        .string()
        .refine((v) => timeToMinutes(v) >= 0, {
          message: t('Check-in opening time must be between 00:00 and 23:59'),
        }),
      minAmount: z.coerce.number().min(0),
      maxAmount: z.coerce.number().min(0),
      fixedAmount: z.coerce.number().min(0),
      randomMode: z.boolean(),
    })
    .superRefine((values, ctx) => {
      if (values.randomMode && values.maxAmount < values.minAmount) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['maxAmount'],
          message: t(
            'Maximum check-in reward must be greater than or equal to minimum'
          ),
        })
      }
    })

type Values = z.infer<ReturnType<typeof createSchema>>

export function CheckinSettingsSection({
  defaultValues,
}: {
  defaultValues: {
    enabled: boolean
    minQuota: number
    maxQuota: number
    fixedQuota: number
    randomMode: boolean
    rewardType: 'permanent' | 'temporary'
    availableFromMinutes: number
    quotaPerUnit?: number
  }
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { status } = useStatus()
  const schema = createSchema(t)
  const quotaPerUnit = normalizeQuotaPerUnit(defaultValues.quotaPerUnit)
  const timezone = status?.checkin_timezone as string | undefined

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: defaultValues.enabled,
      rewardType: defaultValues.rewardType,
      availableFrom: minutesToTime(defaultValues.availableFromMinutes),
      minAmount: quotaToUsdAmount(defaultValues.minQuota, quotaPerUnit),
      maxAmount: quotaToUsdAmount(defaultValues.maxQuota, quotaPerUnit),
      fixedAmount: quotaToUsdAmount(defaultValues.fixedQuota, quotaPerUnit),
      randomMode: defaultValues.randomMode,
    },
  })

  const saveMutation = useMutation({
    mutationFn: updateCheckinSetting,
    onSuccess: (data) => {
      if (data.success) {
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        // 签到开关影响全局状态（checkin_enabled），保存后一并刷新
        queryClient.invalidateQueries({ queryKey: ['status'] })
        toast.success(i18next.t('Check-in settings saved successfully'))
      } else {
        toast.error(
          data.message || i18next.t('Failed to save check-in settings')
        )
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to save check-in settings'))
    },
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')
  const randomMode = form.watch('randomMode')
  const rewardType = form.watch('rewardType')

  async function onSubmit(values: Values) {
    const minQuota = usdAmountToQuota(values.minAmount, quotaPerUnit)
    const maxQuota = usdAmountToQuota(values.maxAmount, quotaPerUnit)
    const fixedQuota = usdAmountToQuota(values.fixedAmount, quotaPerUnit)

    if (minQuota > MAX_QUOTA || maxQuota > MAX_QUOTA || fixedQuota > MAX_QUOTA) {
      toast.error(t('Check-in reward exceeds the allowed maximum'))
      return
    }

    const result = await saveMutation.mutateAsync({
      enabled: values.enabled,
      min_quota: minQuota,
      max_quota: maxQuota,
      fixed_quota: fixedQuota,
      random_mode: values.randomMode,
      reward_type: values.rewardType,
      available_from_minutes: timeToMinutes(values.availableFrom),
    })
    // 仅保存成功才清除 dirty 状态；后端拒绝时保留表单，避免用户误以为已保存
    if (result?.success) {
      form.reset(values)
    }
  }

  return (
    <SettingsSection title={t('Check-in Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={saveMutation.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save check-in settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable check-in feature')}</FormLabel>
                  <FormDescription>
                    {t('Allow users to check in daily for quota rewards')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={saveMutation.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled && (
            <>
              <FormField
                control={form.control}
                name='rewardType'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Check-in reward type')}</FormLabel>
                    <FormControl>
                      <ToggleGroup
                        value={[field.value]}
                        onValueChange={(value) => {
                          const next = value.find((v) => v !== field.value)
                          if (next) field.onChange(next)
                        }}
                        aria-label={t('Check-in reward type')}
                        variant='outline'
                        size='lg'
                        spacing={2}
                        className='grid w-full grid-cols-2 gap-2 sm:max-w-md'
                      >
                        <ToggleGroupItem
                          value='permanent'
                          className='h-auto min-h-11 w-full px-3 text-sm font-medium'
                        >
                          {t('Permanent balance')}
                        </ToggleGroupItem>
                        <ToggleGroupItem
                          value='temporary'
                          className='h-auto min-h-11 w-full px-3 text-sm font-medium'
                        >
                          {t("Today's limited-time quota")}
                        </ToggleGroupItem>
                      </ToggleGroup>
                    </FormControl>
                    {rewardType === 'temporary' && (
                      <FormDescription className='max-w-xl whitespace-normal'>
                        {t(
                          'Limited-time quota does not increase the permanent balance and expires automatically at midnight.'
                        )}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='availableFrom'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Check-in opening time')}</FormLabel>
                    <FormControl>
                      <div className='flex items-center gap-2'>
                        <Clock className='text-muted-foreground size-4' />
                        <Input
                          type='time'
                          className='w-40'
                          {...field}
                        />
                      </div>
                    </FormControl>
                    <FormDescription className='max-w-xl whitespace-normal'>
                      {timezone
                        ? t('Check-in opens daily at this time (server timezone: {{timezone}})', {
                            timezone,
                          })
                        : t('Check-in opens daily at this time')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='randomMode'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Random quota mode')}</FormLabel>
                      <FormDescription>
                        {t('Use random check-in quota rewards')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={saveMutation.isPending || isSubmitting}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />

              {randomMode ? (
                <div className='grid gap-6 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='minAmount'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Minimum check-in reward (USD)')}
                        </FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            step='0.000001'
                            placeholder='1'
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Minimum USD amount awarded for check-in')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='maxAmount'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Maximum check-in reward (USD)')}
                        </FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            step='0.000001'
                            placeholder='5'
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Maximum USD amount awarded for check-in')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              ) : (
                <FormField
                  control={form.control}
                  name='fixedAmount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Fixed check-in reward (USD)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          step='0.000001'
                          placeholder='1'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Fixed USD amount awarded for check-in')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
