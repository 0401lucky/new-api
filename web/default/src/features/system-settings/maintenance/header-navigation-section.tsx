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
import { type DragEvent, useEffect, useMemo, useState } from 'react'
import * as z from 'zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { ExternalLink, GripVertical, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
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
import { Switch } from '@/components/ui/switch'
import {
  SettingsControlChildren,
  SettingsForm,
  SettingsSwitchContent,
  SettingsControlGroup,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  HEADER_NAV_DEFAULT,
  type HeaderNavModulesConfig,
  isAllowedHeaderNavHref,
  serializeHeaderNavModules,
} from './config'

const customLinkSchema = z.object({
  id: z.string().optional(),
  title: z.string().trim().min(1).max(32),
  href: z.string().trim().min(1).max(512).refine(isAllowedHeaderNavHref, {
    message: 'Use an http(s) URL or an internal path starting with /',
  }),
  enabled: z.boolean(),
  requireAuth: z.boolean(),
  openInNewTab: z.boolean(),
})

const headerNavSchema = z.object({
  home: z.boolean(),
  console: z.boolean(),
  pricingEnabled: z.boolean(),
  pricingRequireAuth: z.boolean(),
  rankingsEnabled: z.boolean(),
  rankingsRequireAuth: z.boolean(),
  modelHealthEnabled: z.boolean(),
  modelHealthRequireAuth: z.boolean(),
  docs: z.boolean(),
  about: z.boolean(),
  custom_links: z.array(customLinkSchema).max(8),
})

type HeaderNavFormValues = z.infer<typeof headerNavSchema>

const CUSTOM_LINK_DRAG_TYPE = 'application/x-header-nav-custom-link'

type HeaderNavigationSectionProps = {
  config: HeaderNavModulesConfig
  initialSerialized: string
}

const toFormValues = (config: HeaderNavModulesConfig): HeaderNavFormValues => ({
  home:
    config.home === undefined ? HEADER_NAV_DEFAULT.home : Boolean(config.home),
  console:
    config.console === undefined
      ? HEADER_NAV_DEFAULT.console
      : Boolean(config.console),
  pricingEnabled:
    config.pricing?.enabled === undefined
      ? HEADER_NAV_DEFAULT.pricing.enabled
      : Boolean(config.pricing.enabled),
  pricingRequireAuth:
    config.pricing?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.pricing.requireAuth
      : Boolean(config.pricing.requireAuth),
  rankingsEnabled:
    config.rankings?.enabled === undefined
      ? HEADER_NAV_DEFAULT.rankings.enabled
      : Boolean(config.rankings.enabled),
  rankingsRequireAuth:
    config.rankings?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.rankings.requireAuth
      : Boolean(config.rankings.requireAuth),
  modelHealthEnabled:
    config.model_health?.enabled === undefined
      ? HEADER_NAV_DEFAULT.model_health.enabled
      : Boolean(config.model_health.enabled),
  modelHealthRequireAuth:
    config.model_health?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.model_health.requireAuth
      : Boolean(config.model_health.requireAuth),
  docs:
    config.docs === undefined ? HEADER_NAV_DEFAULT.docs : Boolean(config.docs),
  about:
    config.about === undefined
      ? HEADER_NAV_DEFAULT.about
      : Boolean(config.about),
  custom_links: (config.custom_links ?? []).map((link, index) => ({
    id: link.id || `custom-${index}`,
    title: link.title,
    href: link.href,
    enabled: link.enabled,
    requireAuth: link.requireAuth,
    openInNewTab: link.openInNewTab,
  })),
})

export function HeaderNavigationSection({
  config,
  initialSerialized,
}: HeaderNavigationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(() => toFormValues(config), [config])
  const [draggedLinkIndex, setDraggedLinkIndex] = useState<number | null>(null)
  const [dragOverLinkIndex, setDragOverLinkIndex] = useState<number | null>(
    null
  )

  const form = useForm<HeaderNavFormValues>({
    resolver: zodResolver(headerNavSchema),
    defaultValues: formDefaults,
  })
  const customLinks = useFieldArray({
    control: form.control,
    name: 'custom_links',
    keyName: 'fieldKey',
  })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: HeaderNavFormValues) => {
    const payload: HeaderNavModulesConfig = {
      ...config,
      home: values.home,
      console: values.console,
      docs: values.docs,
      about: values.about,
      pricing: {
        ...(config.pricing ?? HEADER_NAV_DEFAULT.pricing),
        enabled: values.pricingEnabled,
        requireAuth: values.pricingRequireAuth,
      },
      rankings: {
        ...(config.rankings ?? HEADER_NAV_DEFAULT.rankings),
        enabled: values.rankingsEnabled,
        requireAuth: values.rankingsRequireAuth,
      },
      model_health: {
        ...(config.model_health ?? HEADER_NAV_DEFAULT.model_health),
        enabled: values.modelHealthEnabled,
        requireAuth: values.modelHealthRequireAuth,
      },
      custom_links: values.custom_links.map((link, index) => ({
        id: link.id || `custom-${index}-${Date.now()}`,
        title: link.title.trim(),
        href: link.href.trim(),
        enabled: link.enabled,
        requireAuth: link.requireAuth,
        openInNewTab: link.openInNewTab,
      })),
    }

    const serialized = serializeHeaderNavModules(payload)
    if (serialized === initialSerialized) {
      return
    }

    await updateOption.mutateAsync({
      key: 'HeaderNavModules',
      value: serialized,
    })
  }

  const resetToDefault = () => {
    form.reset(toFormValues(HEADER_NAV_DEFAULT))
  }

  const addCustomLink = () => {
    customLinks.append({
      id: `custom-${Date.now()}`,
      title: '',
      href: 'https://',
      enabled: true,
      requireAuth: false,
      openInNewTab: true,
    })
  }

  const clearDragState = () => {
    setDraggedLinkIndex(null)
    setDragOverLinkIndex(null)
  }

  const handleCustomLinkDragStart = (
    event: DragEvent<HTMLButtonElement>,
    index: number
  ) => {
    setDraggedLinkIndex(index)
    setDragOverLinkIndex(index)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData(CUSTOM_LINK_DRAG_TYPE, String(index))
  }

  const handleCustomLinkDragOver = (
    event: DragEvent<HTMLDivElement>,
    index: number
  ) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    setDragOverLinkIndex(index)
  }

  const handleCustomLinkDrop = (
    event: DragEvent<HTMLDivElement>,
    targetIndex: number
  ) => {
    event.preventDefault()
    const rawSourceIndex = event.dataTransfer.getData(CUSTOM_LINK_DRAG_TYPE)
    const sourceIndex =
      draggedLinkIndex ?? (rawSourceIndex ? Number(rawSourceIndex) : NaN)

    if (
      Number.isInteger(sourceIndex) &&
      sourceIndex >= 0 &&
      sourceIndex < customLinks.fields.length &&
      sourceIndex !== targetIndex
    ) {
      customLinks.move(sourceIndex, targetIndex)
    }

    clearDragState()
  }

  const simpleModules: Array<{
    key: keyof HeaderNavFormValues
    title: string
    description: string
  }> = [
    {
      key: 'home',
      title: t('Home'),
      description: t('Landing page with system overview.'),
    },
    {
      key: 'console',
      title: t('Console'),
      description: t('User dashboard and quota controls.'),
    },
    {
      key: 'docs',
      title: t('Docs'),
      description: t('Documentation or external knowledge base.'),
    },
    {
      key: 'about',
      title: t('About'),
      description: t('Static page describing the platform.'),
    },
  ]

  const accessModules: Array<{
    enabledKey: keyof HeaderNavFormValues
    requireAuthKey: keyof HeaderNavFormValues
    requireAuthDependsOn:
      | 'pricingEnabled'
      | 'rankingsEnabled'
      | 'modelHealthEnabled'
    title: string
    description: string
    requireAuthTitle: string
    requireAuthDescription: string
  }> = [
    {
      enabledKey: 'pricingEnabled',
      requireAuthKey: 'pricingRequireAuth',
      requireAuthDependsOn: 'pricingEnabled',
      title: t('Model Square'),
      description: t('Public model catalog and pricing page.'),
      requireAuthTitle: t('Require login to view models'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the pricing directory.'
      ),
    },
    {
      enabledKey: 'rankingsEnabled',
      requireAuthKey: 'rankingsRequireAuth',
      requireAuthDependsOn: 'rankingsEnabled',
      title: t('Rankings'),
      description: t('Public rankings page based on live usage data.'),
      requireAuthTitle: t('Require login to view rankings'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the rankings page.'
      ),
    },
    {
      enabledKey: 'modelHealthEnabled',
      requireAuthKey: 'modelHealthRequireAuth',
      requireAuthDependsOn: 'modelHealthEnabled',
      title: t('Model health'),
      description: t('Public model health status and trend overview.'),
      requireAuthTitle: t('Require login to view model health'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the model health page.'
      ),
    },
  ]

  return (
    <SettingsSection title={t('Header navigation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={resetToDefault}
            isSaving={updateOption.isPending}
            resetLabel='Reset to default'
            saveLabel='Save navigation'
          />
          <div className='grid gap-4 md:grid-cols-2'>
            {simpleModules.map((module) => (
              <FormField
                key={module.key}
                control={form.control}
                name={module.key}
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{module.title}</FormLabel>
                      <FormDescription>{module.description}</FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={Boolean(field.value)}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />
            ))}
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            {accessModules.map((module) => (
              <SettingsControlGroup key={module.enabledKey}>
                <FormField
                  control={form.control}
                  name={module.enabledKey}
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{module.title}</FormLabel>
                        <FormDescription>{module.description}</FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </SettingsSwitchItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name={module.requireAuthKey}
                  render={({ field }) => (
                    <SettingsControlChildren>
                      <SettingsSwitchItem className='border-b-0 py-2'>
                        <SettingsSwitchContent>
                          <FormLabel>{module.requireAuthTitle}</FormLabel>
                          <FormDescription>
                            {module.requireAuthDescription}
                          </FormDescription>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={Boolean(field.value)}
                            onCheckedChange={field.onChange}
                            disabled={
                              !Boolean(form.watch(module.requireAuthDependsOn))
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </SettingsSwitchItem>
                    </SettingsControlChildren>
                  )}
                />
              </SettingsControlGroup>
            ))}
          </div>

          <SettingsControlGroup>
            <div className='flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
              <SettingsSwitchContent>
                <div className='text-sm font-medium'>
                  {t('Custom navigation links')}
                </div>
                <p className='text-muted-foreground text-sm'>
                  {t('Add external or internal links to the public header.')}
                </p>
              </SettingsSwitchContent>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={addCustomLink}
                disabled={customLinks.fields.length >= 8}
              >
                <Plus data-icon='inline-start' />
                <span>{t('Add link')}</span>
              </Button>
            </div>

            {customLinks.fields.length === 0 ? (
              <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-4 text-sm'>
                {t('No custom navigation links configured.')}
              </div>
            ) : (
              <div className='flex flex-col gap-3'>
                {customLinks.fields.map((field, index) => (
                  <div
                    key={field.fieldKey}
                    onDragOver={(event) =>
                      handleCustomLinkDragOver(event, index)
                    }
                    onDragLeave={() => setDragOverLinkIndex(null)}
                    onDrop={(event) => handleCustomLinkDrop(event, index)}
                    className={cn(
                      'bg-background rounded-lg border px-3 py-3 transition-[box-shadow,opacity]',
                      draggedLinkIndex === index && 'opacity-60',
                      dragOverLinkIndex === index &&
                        draggedLinkIndex !== index &&
                        'ring-ring ring-offset-background ring-2 ring-offset-2'
                    )}
                  >
                    <div className='mb-3 flex items-center justify-between gap-2'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          draggable
                          onDragStart={(event) =>
                            handleCustomLinkDragStart(event, index)
                          }
                          onDragEnd={clearDragState}
                          className='cursor-grab active:cursor-grabbing'
                          aria-label={`${t('Custom link')} ${index + 1}`}
                        >
                          <GripVertical />
                        </Button>
                        <div className='truncate text-sm font-medium'>
                          {t('Custom link')} {index + 1}
                        </div>
                      </div>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        onClick={() => customLinks.remove(index)}
                        aria-label={t('Remove link')}
                      >
                        <Trash2 />
                      </Button>
                    </div>

                    <div className='grid gap-3 md:grid-cols-2'>
                      <FormField
                        control={form.control}
                        name={`custom_links.${index}.title`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Link title')}</FormLabel>
                            <FormControl>
                              <Input placeholder={t('My store')} {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name={`custom_links.${index}.href`}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Link URL')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder='https://shop.example.com'
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>

                    <SettingsControlChildren className='mt-3 grid gap-3 md:grid-cols-3'>
                      <FormField
                        control={form.control}
                        name={`custom_links.${index}.enabled`}
                        render={({ field }) => (
                          <SettingsSwitchItem className='border-b-0 py-2'>
                            <SettingsSwitchContent>
                              <FormLabel>{t('Enabled')}</FormLabel>
                              <FormDescription>
                                {t('Show this link in the header.')}
                              </FormDescription>
                            </SettingsSwitchContent>
                            <FormControl>
                              <Switch
                                checked={Boolean(field.value)}
                                onCheckedChange={field.onChange}
                              />
                            </FormControl>
                          </SettingsSwitchItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`custom_links.${index}.requireAuth`}
                        render={({ field }) => (
                          <SettingsSwitchItem className='border-b-0 py-2'>
                            <SettingsSwitchContent>
                              <FormLabel>{t('Require login')}</FormLabel>
                              <FormDescription>
                                {t('Visitors must sign in before opening it.')}
                              </FormDescription>
                            </SettingsSwitchContent>
                            <FormControl>
                              <Switch
                                checked={Boolean(field.value)}
                                onCheckedChange={field.onChange}
                              />
                            </FormControl>
                          </SettingsSwitchItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name={`custom_links.${index}.openInNewTab`}
                        render={({ field }) => (
                          <SettingsSwitchItem className='border-b-0 py-2'>
                            <SettingsSwitchContent>
                              <FormLabel className='inline-flex items-center gap-1.5'>
                                <ExternalLink className='size-3.5' />
                                {t('New tab')}
                              </FormLabel>
                              <FormDescription>
                                {t('Open external links in a new tab.')}
                              </FormDescription>
                            </SettingsSwitchContent>
                            <FormControl>
                              <Switch
                                checked={Boolean(field.value)}
                                onCheckedChange={field.onChange}
                              />
                            </FormControl>
                          </SettingsSwitchItem>
                        )}
                      />
                    </SettingsControlChildren>
                  </div>
                ))}
              </div>
            )}
          </SettingsControlGroup>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
