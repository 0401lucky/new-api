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
import { useEffect, useMemo, useState } from 'react'
import * as z from 'zod'
import { useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { MultiSelect, type Option } from '@/components/multi-select'
import { getAllLogs } from '@/features/usage-logs/api'
import { DetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { parseLogOther } from '@/features/usage-logs/lib/format'
import { getGroups } from '@/features/users/api'
import { getPromptCheckRules, getUpstreamChannels } from '../api'
import {
  SettingsForm,
  SettingsFormGridItem,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { PromptCheckRule } from '../types'

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
  PromptCheckDisabledRules: z.string().optional(),
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

const WHITELIST_VALUE_SEPARATOR = /[\n\r,，;；]+/

function splitWhitelistValue(value?: string | null): string[] {
  if (!value) return []

  const result: string[] = []
  const seen = new Set<string>()

  for (const rawItem of value.split(WHITELIST_VALUE_SEPARATOR)) {
    const item = rawItem.trim()
    if (!item || seen.has(item)) continue
    seen.add(item)
    result.push(item)
  }

  return result
}

function formatWhitelistValue(values: string[]): string {
  return splitWhitelistValue(values.join('\n')).join('\n')
}

function createTextOptions(values: string[]): Option[] {
  return values.map((value) => ({
    label: value,
    value,
  }))
}

function mergeSelectedOptions(
  options: Option[],
  selectedValues: string[],
  getFallbackLabel: (value: string) => string = (value) => value
): Option[] {
  const optionMap = new Map(options.map((option) => [option.value, option]))

  for (const value of selectedValues) {
    if (!optionMap.has(value)) {
      optionMap.set(value, {
        label: getFallbackLabel(value),
        value,
      })
    }
  }

  return Array.from(optionMap.values())
}

function formatLogTime(timestamp?: number): string {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function getActionBadgeVariant(action?: string) {
  if (action === 'block') return 'destructive' as const
  if (action === 'warn') return 'secondary' as const
  return 'outline' as const
}

function getKeywordCount(value?: string | null): number {
  return splitWhitelistValue(value).length
}

function PromptCheckTriggerLogsPanel() {
  const { t } = useTranslation()
  const [selectedLog, setSelectedLog] = useState<UsageLog | null>(null)
  const logsQuery = useQuery({
    queryKey: ['prompt-check-trigger-logs'],
    queryFn: () =>
      getAllLogs({
        type: 5,
        page_size: 20,
        prompt_check: true,
      }),
    staleTime: 30 * 1000,
  })

  const logs = (logsQuery.data?.data?.items ?? []) as UsageLog[]

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-wrap items-center gap-2'>
          <Badge variant='secondary'>
            {t('{{count}} records', { count: logs.length })}
          </Badge>
          <Badge variant='outline'>{t('Error logs')}</Badge>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => void logsQuery.refetch()}
          disabled={logsQuery.isFetching}
        >
          {logsQuery.isFetching ? t('Refreshing...') : t('Refresh')}
        </Button>
      </div>

      <div className='min-w-0 overflow-hidden rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('User')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Action')}</TableHead>
              <TableHead>{t('Score')}</TableHead>
              <TableHead>{t('Matched rules')}</TableHead>
              <TableHead className='text-right'>{t('Action')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {logsQuery.isLoading ? (
              <TableRow>
                <TableCell colSpan={8} className='h-24 text-center'>
                  {t('Loading')}
                </TableCell>
              </TableRow>
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className='h-24 text-center'>
                  {t('No prompt check trigger logs')}
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log) => {
                const other = parseLogOther(log.other)
                const promptCheck = other?.prompt_check
                const match = promptCheck?.matches?.[0]
                const action =
                  promptCheck?.action ||
                  (other?.reject_reason === 'prompt_check' ? 'block' : '-')

                return (
                  <TableRow key={log.id}>
                    <TableCell>{formatLogTime(log.created_at)}</TableCell>
                    <TableCell>{log.username || `#${log.user_id}`}</TableCell>
                    <TableCell className='max-w-48 truncate'>
                      {log.model_name || '-'}
                    </TableCell>
                    <TableCell>
                      {log.channel
                        ? `#${log.channel}${log.channel_name ? ` ${log.channel_name}` : ''}`
                        : '-'}
                    </TableCell>
                    <TableCell>
                      <Badge variant={getActionBadgeVariant(action)}>
                        {action}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {promptCheck
                        ? `${promptCheck.score ?? 0}/${promptCheck.threshold ?? '-'}`
                        : '-'}
                    </TableCell>
                    <TableCell className='max-w-72 truncate'>
                      {match
                        ? `${match.name || '-'}${match.matched ? `: ${match.matched}` : ''}`
                        : '-'}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => setSelectedLog(log)}
                      >
                        {t('View')}
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {selectedLog && (
        <DetailsDialog
          log={selectedLog}
          isAdmin
          open={Boolean(selectedLog)}
          onOpenChange={(open) => {
            if (!open) setSelectedLog(null)
          }}
        />
      )}
    </div>
  )
}

function PromptCheckRuleManagementPanel(props: {
  disabledRulesValue?: string | null
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const rulesQuery = useQuery({
    queryKey: ['prompt-check-rules'],
    queryFn: getPromptCheckRules,
    staleTime: 5 * 60 * 1000,
  })
  const [disabledRules, setDisabledRules] = useState<string[]>(() =>
    splitWhitelistValue(props.disabledRulesValue)
  )

  useEffect(() => {
    setDisabledRules(splitWhitelistValue(props.disabledRulesValue))
  }, [props.disabledRulesValue])

  const rules = rulesQuery.data?.data ?? []
  const disabledRuleSet = useMemo(
    () => new Set(disabledRules.map((rule) => rule.toLowerCase())),
    [disabledRules]
  )
  const strictRuleCount = rules.filter((rule) => rule.strict).length

  const handleRuleEnabledChange = async (
    rule: PromptCheckRule,
    enabled: boolean
  ) => {
    const next = enabled
      ? disabledRules.filter(
          (name) => name.toLowerCase() !== rule.name.toLowerCase()
        )
      : [...disabledRules, rule.name]
    const normalizedNext = splitWhitelistValue(next.join('\n'))

    setDisabledRules(normalizedNext)
    await updateOption.mutateAsync({
      key: 'PromptCheckDisabledRules',
      value: formatWhitelistValue(normalizedNext),
    })
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant='secondary'>
          {t('{{count}} built-in rules', { count: rules.length })}
        </Badge>
        <Badge variant='outline'>
          {t('{{count}} strict rules', { count: strictRuleCount })}
        </Badge>
        <Badge variant='outline'>
          {t('{{count}} disabled rules', { count: disabledRules.length })}
        </Badge>
      </div>

      <div className='min-w-0 overflow-hidden rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Rule')}</TableHead>
              <TableHead>{t('Category')}</TableHead>
              <TableHead>{t('Weight')}</TableHead>
              <TableHead>{t('Strict')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Enabled')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rulesQuery.isLoading ? (
              <TableRow>
                <TableCell colSpan={6} className='h-24 text-center'>
                  {t('Loading')}
                </TableCell>
              </TableRow>
            ) : rules.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className='h-24 text-center'>
                  {t('No rules found')}
                </TableCell>
              </TableRow>
            ) : (
              rules.map((rule) => {
                const enabled = !disabledRuleSet.has(rule.name.toLowerCase())

                return (
                  <TableRow key={rule.name}>
                    <TableCell className='max-w-80 whitespace-normal'>
                      <div className='flex min-w-0 flex-col gap-1'>
                        <span className='font-medium'>{rule.name}</span>
                        {rule.pattern && (
                          <span className='text-muted-foreground line-clamp-2 font-mono text-xs break-all'>
                            {rule.pattern}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>{rule.category || '-'}</TableCell>
                    <TableCell>{rule.weight}</TableCell>
                    <TableCell>{rule.strict ? t('Yes') : t('No')}</TableCell>
                    <TableCell>
                      <Badge variant={enabled ? 'secondary' : 'outline'}>
                        {enabled ? t('Enabled') : t('Disabled')}
                      </Badge>
                    </TableCell>
                    <TableCell className='text-right'>
                      <Switch
                        checked={enabled}
                        disabled={updateOption.isPending}
                        aria-label={t('Toggle rule')}
                        onCheckedChange={(checked) =>
                          void handleRuleEnabledChange(rule, checked)
                        }
                      />
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
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

  const groupWhitelistValue = useWatch({
    control: form.control,
    name: 'PromptCheckGroupWhitelist',
  })
  const channelWhitelistValue = useWatch({
    control: form.control,
    name: 'PromptCheckChannelWhitelist',
  })

  const groupsQuery = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
    staleTime: 5 * 60 * 1000,
  })
  const channelsQuery = useQuery({
    queryKey: ['upstream-channels'],
    queryFn: getUpstreamChannels,
    staleTime: 5 * 60 * 1000,
  })

  const selectedGroupWhitelist = useMemo(
    () => splitWhitelistValue(groupWhitelistValue),
    [groupWhitelistValue]
  )
  const selectedChannelWhitelist = useMemo(
    () => splitWhitelistValue(channelWhitelistValue),
    [channelWhitelistValue]
  )

  const groupWhitelistOptions = useMemo(
    () =>
      mergeSelectedOptions(
        createTextOptions(groupsQuery.data?.data ?? []),
        selectedGroupWhitelist
      ),
    [groupsQuery.data?.data, selectedGroupWhitelist]
  )

  const channelWhitelistOptions = useMemo(() => {
    const options =
      channelsQuery.data?.data.map((channel) => {
        const value = String(channel.id)
        const description = channel.name || channel.base_url

        return {
          label: description ? `#${value} ${description}` : `#${value}`,
          value,
        }
      }) ?? []

    return mergeSelectedOptions(
      options,
      selectedChannelWhitelist,
      (value) => `#${value}`
    )
  }, [channelsQuery.data?.data, selectedChannelWhitelist])

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
      <Tabs defaultValue='overview' className='flex flex-col gap-4'>
        <TabsList className='grid w-full grid-cols-3'>
          <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
          <TabsTrigger value='trigger-logs'>{t('Trigger Logs')}</TabsTrigger>
          <TabsTrigger value='rules'>{t('Rule Management')}</TabsTrigger>
        </TabsList>

        <div className='flex flex-wrap items-center gap-2'>
          <Badge
            variant={
              defaultValues.CheckSensitiveEnabled ? 'secondary' : 'outline'
            }
          >
            {defaultValues.CheckSensitiveEnabled
              ? t('Prompt check enabled')
              : t('Prompt check disabled')}
          </Badge>
          <Badge variant='outline'>
            {t('{{count}} keywords', {
              count: getKeywordCount(defaultValues.SensitiveWords),
            })}
          </Badge>
          <Badge variant='outline'>
            {t('{{count}} disabled rules', {
              count: splitWhitelistValue(defaultValues.PromptCheckDisabledRules)
                .length,
            })}
          </Badge>
        </div>

        <TabsContent value='overview'>
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
                        <FormLabel>
                          {t('Inspect prompts before relay')}
                        </FormLabel>
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
                      <MultiSelect
                        options={groupWhitelistOptions}
                        selected={splitWhitelistValue(field.value)}
                        onChange={(values) =>
                          field.onChange(formatWhitelistValue(values))
                        }
                        placeholder={t('Select groups to skip prompt checks')}
                        allowCreate
                        createLabel='Add group "{{value}}"'
                        emptyText={t('No groups found')}
                        maxVisibleChips={6}
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
                      <MultiSelect
                        options={channelWhitelistOptions}
                        selected={splitWhitelistValue(field.value)}
                        onChange={(values) =>
                          field.onChange(formatWhitelistValue(values))
                        }
                        placeholder={t('Select channels to skip prompt checks')}
                        allowCreate
                        createLabel='Add channel ID "{{value}}"'
                        emptyText={t('No channels found')}
                        maxVisibleChips={6}
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
                          <FormLabel>
                            {t('Fail closed on review error')}
                          </FormLabel>
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
        </TabsContent>

        <TabsContent value='trigger-logs'>
          <PromptCheckTriggerLogsPanel />
        </TabsContent>

        <TabsContent value='rules'>
          <PromptCheckRuleManagementPanel
            disabledRulesValue={defaultValues.PromptCheckDisabledRules}
          />
        </TabsContent>
      </Tabs>
    </SettingsSection>
  )
}
