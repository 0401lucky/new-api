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
import { useEffect } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsForm,
  SettingsFormGridItem,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const sensitiveSchema = z.object({
  CheckSensitiveEnabled: z.boolean(),
  CheckSensitiveOnPromptEnabled: z.boolean(),
  SensitiveWords: z.string().optional(),
  PromptCheckMode: z.enum(['monitor', 'warn', 'block']),
  PromptCheckThreshold: z.number().int().min(1).max(500),
  PromptCheckStrictThreshold: z.number().int().min(1).max(1000),
  PromptCheckLogMatchesEnabled: z.boolean(),
  PromptCheckMaxTextLength: z.number().int().min(1024).max(1048576),
  PromptCheckModelScope: z.string().optional(),
  PromptCheckGroupWhitelist: z.string().optional(),
  PromptCheckChannelWhitelist: z.string().optional(),
  PromptCheckAPIReviewEnabled: z.boolean(),
  PromptCheckAPIReviewModel: z.string().optional(),
  PromptCheckAPIReviewBaseURL: z.string().optional(),
  PromptCheckAPIReviewKey: z.string().optional(),
  PromptCheckAPIReviewTimeoutMS: z.number().int().min(500).max(30000),
  PromptCheckAPIReviewFailClosedEnabled: z.boolean(),
})

type SensitiveFormValues = z.infer<typeof sensitiveSchema>

type SensitiveWordsSectionProps = {
  defaultValues: SensitiveFormValues
}

export function SensitiveWordsSection({
  defaultValues,
}: SensitiveWordsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<SensitiveFormValues>({
    resolver: zodResolver(sensitiveSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: SensitiveFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof SensitiveFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Prompt Check')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save prompt check'
          />
          <div className='flex flex-col gap-4'>
            <FormField
              control={form.control}
              name='CheckSensitiveEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable prompt check')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Checks prompts before they reach upstream models to reduce jailbreak, reverse engineering, and NSFW abuse.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='CheckSensitiveOnPromptEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Inspect prompts before relay')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Keeps the check in the relay preflight stage so blocked requests never reach the provider.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='PromptCheckLogMatchesEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Record matched logs')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Writes matched prompt check summaries to error logs for audit and manual review.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='PromptCheckMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Processing mode')}</FormLabel>
                <Select value={field.value} onValueChange={field.onChange}>
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select mode')} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='monitor'>
                        {t('Monitor only')}
                      </SelectItem>
                      <SelectItem value='warn'>{t('Warn only')}</SelectItem>
                      <SelectItem value='block'>
                        {t('Block request')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Monitor records matches, warn adds a response header, and block rejects matched requests.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckThreshold'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Match threshold')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={500}
                    name={field.name}
                    ref={field.ref}
                    value={field.value ?? ''}
                    onBlur={field.onBlur}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value)
                      )
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Rule scores greater than or equal to this value are considered matched.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckStrictThreshold'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Strict threshold')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    max={1000}
                    name={field.name}
                    ref={field.ref}
                    value={field.value ?? ''}
                    onBlur={field.onBlur}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value)
                      )
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Strict rules can trigger even when defensive context lowers the normal score.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckMaxTextLength'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Maximum checked characters')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1024}
                    max={1048576}
                    name={field.name}
                    ref={field.ref}
                    value={field.value ?? ''}
                    onBlur={field.onBlur}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value)
                      )
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Long prompts are checked from the head and tail to avoid excessive memory usage.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckModelScope'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Model scope')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={'gpt*\no*\nchatgpt*'}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'One wildcard pattern per line. Leave empty to check all models.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckGroupWhitelist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Group whitelist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={t('Enter one group per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Requests from these user groups skip prompt checks.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckChannelWhitelist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Channel whitelist')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={t('Enter one channel ID per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Requests routed to these channel IDs skip prompt checks.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='SensitiveWords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={t('Enter one keyword per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Each line is treated as a strict keyword and immediately raises the prompt check score.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <SettingsFormGridItem span='full'>
            <div className='border-border/70 flex flex-col gap-4 border-t pt-4'>
              <FormField
                control={form.control}
                name='PromptCheckAPIReviewEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable API review')}</FormLabel>
                      <FormDescription>
                        {t(
                          'After local rules match, send the extracted prompt to a moderation API for a second opinion.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />

              <FormField
                control={form.control}
                name='PromptCheckAPIReviewFailClosedEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Fail closed on review error')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, review API errors block matched requests instead of allowing them.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </div>
          </SettingsFormGridItem>

          <FormField
            control={form.control}
            name='PromptCheckAPIReviewModel'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Review model')}</FormLabel>
                <FormControl>
                  <Input placeholder='omni-moderation-latest' {...field} />
                </FormControl>
                <FormDescription>
                  {t('Model used by the moderation API review step.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckAPIReviewTimeoutMS'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Review timeout (ms)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={500}
                    max={30000}
                    name={field.name}
                    ref={field.ref}
                    value={field.value ?? ''}
                    onBlur={field.onBlur}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value)
                      )
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t('Timeout for the moderation API review request.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckAPIReviewBaseURL'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Review Base URL')}</FormLabel>
                <FormControl>
                  <Input placeholder='https://api.openai.com' {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Base URL for the moderation API. The /v1/moderations path is added automatically.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='PromptCheckAPIReviewKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Review API Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    placeholder={t('Leave blank to keep existing key')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Stored on the server and hidden from option reads after saving.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
