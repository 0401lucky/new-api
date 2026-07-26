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
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const DEFAULT_QUOTA_PER_USD = 500000

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
    quotaPerUnit?: number
  }
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createSchema(t)
  const quotaPerUnit = normalizeQuotaPerUnit(defaultValues.quotaPerUnit)

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: defaultValues.enabled,
      minAmount: quotaToUsdAmount(defaultValues.minQuota, quotaPerUnit),
      maxAmount: quotaToUsdAmount(defaultValues.maxQuota, quotaPerUnit),
      fixedAmount: quotaToUsdAmount(defaultValues.fixedQuota, quotaPerUnit),
      randomMode: defaultValues.randomMode,
    },
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')
  const randomMode = form.watch('randomMode')

  async function onSubmit(values: Values) {
    const updates: Array<{ key: string; value: string }> = []

    if (values.enabled !== defaultValues.enabled) {
      updates.push({
        key: 'checkin_setting.enabled',
        value: String(values.enabled),
      })
    }

    const minQuota = usdAmountToQuota(values.minAmount, quotaPerUnit)
    const maxQuota = usdAmountToQuota(values.maxAmount, quotaPerUnit)
    const fixedQuota = usdAmountToQuota(values.fixedAmount, quotaPerUnit)

    if (minQuota !== defaultValues.minQuota) {
      updates.push({
        key: 'checkin_setting.min_quota',
        value: String(minQuota),
      })
    }

    if (maxQuota !== defaultValues.maxQuota) {
      updates.push({
        key: 'checkin_setting.max_quota',
        value: String(maxQuota),
      })
    }

    if (fixedQuota !== defaultValues.fixedQuota) {
      updates.push({
        key: 'checkin_setting.fixed_quota',
        value: String(fixedQuota),
      })
    }

    if (values.randomMode !== defaultValues.randomMode) {
      updates.push({
        key: 'checkin_setting.random_mode',
        value: String(values.randomMode),
      })
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset(values)
  }

  return (
    <SettingsSection title={t('Check-in Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
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
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled && (
            <>
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
                        disabled={updateOption.isPending || isSubmitting}
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
