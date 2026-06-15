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
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  ArrowDown,
  ArrowUp,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
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
import { SectionPageLayout } from '@/components/layout'
import { getGroups } from '@/features/users/api'
import {
  createDynamicRatioRule,
  deleteDynamicRatioRule,
  getDynamicRatioRules,
  getDynamicRatioStatus,
  reorderDynamicRatioRules,
  setDynamicRatioEnabled,
  updateDynamicRatioRule,
} from './api'
import type { DynamicRatioRule, DynamicRatioRulePayload } from './types'

type RuleFormState = {
  group: string
  models: string
  concurrency: string
  weekdays: number[]
  start_time: string
  end_time: string
  ratio: string
  priority: string
  enable: boolean
}

const DEFAULT_FORM: RuleFormState = {
  group: '',
  models: '',
  concurrency: '',
  weekdays: [],
  start_time: '',
  end_time: '',
  ratio: '1.5',
  priority: '0',
  enable: true,
}

const WEEKDAYS = [
  { value: 0, label: 'Sun' },
  { value: 1, label: 'Mon' },
  { value: 2, label: 'Tue' },
  { value: 3, label: 'Wed' },
  { value: 4, label: 'Thu' },
  { value: 5, label: 'Fri' },
  { value: 6, label: 'Sat' },
]

const queryKeys = {
  rules: ['dynamic-ratio', 'rules'] as const,
  status: ['dynamic-ratio', 'status'] as const,
  groups: ['dynamic-ratio', 'groups'] as const,
}

function formatRatio(value?: number): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${Number(value.toFixed(4))}x`
}

function parseJsonArray<T>(value: string): T[] {
  if (!value) return []
  const parsed = JSON.parse(value) as unknown
  if (!Array.isArray(parsed)) return []
  return parsed as T[]
}

function ruleToForm(rule: DynamicRatioRule | null): RuleFormState {
  if (!rule) return { ...DEFAULT_FORM }

  let weekdays: number[] = []
  if (rule.weekdays) {
    weekdays = parseJsonArray<number>(rule.weekdays).map((day) => Number(day))
  }

  let models = ''
  if (rule.models) {
    try {
      const parsed = parseJsonArray<string>(rule.models)
      models = parsed.length > 0 ? parsed.join(', ') : ''
    } catch {
      models = rule.models
    }
  }

  return {
    group: rule.group,
    models,
    concurrency: rule.concurrency == null ? '' : String(rule.concurrency),
    weekdays,
    start_time: rule.start_time || '',
    end_time: rule.end_time || '',
    ratio: String(rule.ratio),
    priority: String(rule.priority ?? 0),
    enable: rule.enable !== false,
  }
}

function ruleToPayload(rule: DynamicRatioRule): DynamicRatioRulePayload & {
  id: number
} {
  return {
    id: rule.id,
    enable: rule.enable !== false,
    group: rule.group,
    models: rule.models || '',
    concurrency: rule.concurrency ?? null,
    weekdays: rule.weekdays || '',
    start_time: rule.start_time || '',
    end_time: rule.end_time || '',
    ratio: rule.ratio,
    priority: rule.priority ?? 0,
  }
}

function buildPayload(form: RuleFormState): DynamicRatioRulePayload {
  const group = form.group.trim()
  if (!group) throw new Error('Please select a group')

  const ratio = Number(form.ratio)
  if (!Number.isFinite(ratio) || ratio <= 0) {
    throw new Error('Ratio must be greater than 0')
  }

  const modelsText = form.models.trim()
  const models = modelsText
    ? JSON.stringify(
        modelsText
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean)
      )
    : ''

  const concurrencyText = form.concurrency.trim()
  const concurrency =
    concurrencyText === '' ? null : Number.parseInt(concurrencyText, 10)
  if (
    concurrency !== null &&
    (!Number.isFinite(concurrency) || concurrency <= 0)
  ) {
    throw new Error('Concurrency threshold must be greater than 0')
  }

  const startTime = form.start_time.trim()
  const endTime = form.end_time.trim()
  if ((startTime && !endTime) || (!startTime && endTime)) {
    throw new Error('Start time and end time must be set together')
  }

  const priorityText = form.priority.trim()
  const priority = priorityText === '' ? 0 : Number.parseInt(priorityText, 10)
  if (!Number.isFinite(priority)) {
    throw new Error('Priority must be a number')
  }

  return {
    enable: form.enable,
    group,
    models,
    concurrency,
    weekdays: form.weekdays.length > 0 ? JSON.stringify(form.weekdays) : '',
    start_time: startTime,
    end_time: endTime,
    ratio,
    priority,
  }
}

function formatWeekdays(
  value: string,
  everyDayLabel: string,
  translate: (key: string) => string
): string {
  if (!value) return everyDayLabel
  try {
    const days = parseJsonArray<number>(value)
    if (days.length === 0) return everyDayLabel
    return days
      .map((day) => {
        const weekday = WEEKDAYS.find((item) => item.value === Number(day))
        return weekday ? translate(weekday.label) : String(day)
      })
      .join(', ')
  } catch {
    return value
  }
}

function formatModels(value: string, allModelsLabel: string): string {
  if (!value) return allModelsLabel
  try {
    const models = parseJsonArray<string>(value)
    if (models.length === 0) return allModelsLabel
    return models.join(', ')
  } catch {
    return value || allModelsLabel
  }
}

function ratioVariant(ratio: number) {
  if (ratio > 3) return 'destructive'
  if (ratio > 1.5) return 'secondary'
  return 'outline'
}

function mutationErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback
}

function StatCard(props: {
  title: string
  value: string
  description?: string
}) {
  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardDescription>{props.title}</CardDescription>
        <CardTitle className='text-2xl'>{props.value}</CardTitle>
        {props.description ? (
          <CardDescription>{props.description}</CardDescription>
        ) : null}
      </CardHeader>
    </Card>
  )
}

export function DynamicRatio() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const authUser = useAuthStore((state) => state.auth.user)
  const canEdit = (authUser?.role ?? 0) >= ROLE.SUPER_ADMIN

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<DynamicRatioRule | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DynamicRatioRule | null>(
    null
  )
  const [form, setForm] = useState<RuleFormState>({ ...DEFAULT_FORM })

  const rulesQuery = useQuery({
    queryKey: queryKeys.rules,
    queryFn: getDynamicRatioRules,
  })

  const statusQuery = useQuery({
    queryKey: queryKeys.status,
    queryFn: getDynamicRatioStatus,
  })

  const groupsQuery = useQuery({
    queryKey: queryKeys.groups,
    queryFn: async () => {
      const res = await getGroups()
      if (!res.success) {
        throw new Error(res.message || 'Failed to load groups')
      }
      if (!res.data) {
        throw new Error('Failed to load groups')
      }
      return res.data
    },
  })

  const rules = rulesQuery.data ?? []
  const groups = groupsQuery.data ?? []
  const globalEnabled = Boolean(statusQuery.data?.enabled)
  const isFetching = useMemo(
    () =>
      rulesQuery.isFetching || statusQuery.isFetching || groupsQuery.isFetching,
    [groupsQuery.isFetching, rulesQuery.isFetching, statusQuery.isFetching]
  )

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.rules }),
      queryClient.invalidateQueries({ queryKey: queryKeys.status }),
      queryClient.invalidateQueries({ queryKey: queryKeys.groups }),
    ])
  }

  const setEnabledMutation = useMutation({
    mutationFn: setDynamicRatioEnabled,
    onSuccess: async (_, enabled) => {
      toast.success(
        enabled
          ? t('Dynamic ratio has been enabled')
          : t('Dynamic ratio has been disabled')
      )
      await queryClient.invalidateQueries({ queryKey: queryKeys.status })
    },
    onError: (error) => {
      toast.error(mutationErrorMessage(error, t('Request failed')))
    },
  })

  const saveRuleMutation = useMutation({
    mutationFn: (payload: DynamicRatioRulePayload) =>
      editingRule
        ? updateDynamicRatioRule({ ...payload, id: editingRule.id })
        : createDynamicRatioRule(payload),
    onSuccess: async () => {
      toast.success(editingRule ? t('Updated successfully') : t('Created'))
      setDialogOpen(false)
      setEditingRule(null)
      setForm({ ...DEFAULT_FORM })
      await refreshAll()
    },
    onError: (error) => {
      toast.error(mutationErrorMessage(error, t('Request failed')))
    },
  })

  const updateRuleMutation = useMutation({
    mutationFn: updateDynamicRatioRule,
    onSuccess: async () => {
      toast.success(t('Updated successfully'))
      await refreshAll()
    },
    onError: (error) => {
      toast.error(mutationErrorMessage(error, t('Request failed')))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteDynamicRatioRule,
    onSuccess: async () => {
      toast.success(t('Deleted successfully'))
      setDeleteTarget(null)
      await refreshAll()
    },
    onError: (error) => {
      toast.error(mutationErrorMessage(error, t('Request failed')))
    },
  })

  const reorderMutation = useMutation({
    mutationFn: reorderDynamicRatioRules,
    onSuccess: async () => {
      toast.success(t('Order updated'))
      await refreshAll()
    },
    onError: (error) => {
      toast.error(mutationErrorMessage(error, t('Request failed')))
    },
  })

  const error = rulesQuery.error || statusQuery.error || groupsQuery.error
  const errorMessage =
    error instanceof Error ? error.message : error ? t('Request failed') : ''

  const openCreateDialog = () => {
    setEditingRule(null)
    setForm({ ...DEFAULT_FORM })
    setDialogOpen(true)
  }

  const openEditDialog = (rule: DynamicRatioRule) => {
    try {
      setEditingRule(rule)
      setForm(ruleToForm(rule))
      setDialogOpen(true)
    } catch (error) {
      toast.error(mutationErrorMessage(error, t('Invalid rule')))
    }
  }

  const handleSubmit = () => {
    try {
      saveRuleMutation.mutate(buildPayload(form))
    } catch (error) {
      toast.error(mutationErrorMessage(error, t('Invalid form')))
    }
  }

  const handleMove = (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= rules.length) return
    const reordered = [...rules]
    const current = reordered[index]
    reordered[index] = reordered[nextIndex]
    reordered[nextIndex] = current
    reorderMutation.mutate(reordered.map((rule) => rule.id))
  }

  const toggleWeekday = (day: number, checked: boolean) => {
    setForm((current) => ({
      ...current,
      weekdays: checked
        ? [...current.weekdays, day].sort((a, b) => a - b)
        : current.weekdays.filter((item) => item !== day),
    }))
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Dynamic Ratio')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <div className='flex items-center gap-2 rounded-lg border px-2.5 py-1.5'>
            <span className='text-muted-foreground text-sm'>{t('Global')}</span>
            <Switch
              checked={globalEnabled}
              disabled={
                !canEdit ||
                statusQuery.isLoading ||
                setEnabledMutation.isPending
              }
              onCheckedChange={(checked) => setEnabledMutation.mutate(checked)}
            />
          </div>
          <Button variant='outline' disabled={isFetching} onClick={refreshAll}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
          <Button disabled={!canEdit} onClick={openCreateDialog}>
            <Plus data-icon='inline-start' />
            {t('New rule')}
          </Button>
        </SectionPageLayout.Actions>

        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            {errorMessage ? (
              <Alert variant='destructive'>
                <AlertCircle />
                <AlertTitle>{t('Request failed')}</AlertTitle>
                <AlertDescription>{errorMessage}</AlertDescription>
              </Alert>
            ) : null}

            <div className='grid gap-3 md:grid-cols-4'>
              <StatCard
                title={t('Status')}
                value={globalEnabled ? t('Enabled') : t('Disabled')}
              />
              <StatCard
                title={t('Active ratio')}
                value={formatRatio(statusQuery.data?.active_ratio)}
                description={statusQuery.data?.active_group || undefined}
              />
              <StatCard
                title={t('Rules')}
                value={String(rules.length)}
                description={t('{{count}} enabled', {
                  count: rules.filter((rule) => rule.enable !== false).length,
                })}
              />
              <StatCard
                title={t('Timezone')}
                value={statusQuery.data?.timezone || '-'}
              />
            </div>

            <Card>
              <CardHeader className='border-b'>
                <CardTitle>{t('Dynamic ratio rules')}</CardTitle>
                <CardDescription>
                  {t('{{count}} records', { count: rules.length })}
                </CardDescription>
              </CardHeader>
              <CardContent className='p-0'>
                <div className='overflow-x-auto'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className='w-20'>{t('Enabled')}</TableHead>
                        <TableHead>{t('Group')}</TableHead>
                        <TableHead>{t('Models')}</TableHead>
                        <TableHead>{t('Concurrency')}</TableHead>
                        <TableHead>{t('Weekdays')}</TableHead>
                        <TableHead>{t('Time Range')}</TableHead>
                        <TableHead>{t('Ratio')}</TableHead>
                        <TableHead>{t('Priority')}</TableHead>
                        <TableHead className='text-right'>
                          {t('Actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rulesQuery.isLoading ? (
                        <TableRow>
                          <TableCell colSpan={9} className='h-32 text-center'>
                            {t('Loading')}
                          </TableCell>
                        </TableRow>
                      ) : rules.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={9}>
                            <Empty className='min-h-32 border-0'>
                              <EmptyHeader>
                                <EmptyMedia variant='icon'>
                                  <AlertCircle />
                                </EmptyMedia>
                                <EmptyTitle>
                                  {t('No dynamic ratio rules')}
                                </EmptyTitle>
                              </EmptyHeader>
                            </Empty>
                          </TableCell>
                        </TableRow>
                      ) : (
                        rules.map((rule, index) => (
                          <TableRow key={rule.id}>
                            <TableCell>
                              <Switch
                                size='sm'
                                checked={rule.enable !== false}
                                disabled={
                                  !canEdit || updateRuleMutation.isPending
                                }
                                onCheckedChange={(checked) =>
                                  updateRuleMutation.mutate({
                                    ...ruleToPayload(rule),
                                    enable: checked,
                                  })
                                }
                              />
                            </TableCell>
                            <TableCell>
                              <Badge variant='outline'>{rule.group}</Badge>
                            </TableCell>
                            <TableCell className='max-w-56 truncate'>
                              {formatModels(rule.models, t('All models'))}
                            </TableCell>
                            <TableCell>
                              {rule.concurrency ? (
                                <Badge variant='secondary'>
                                  {rule.concurrency}
                                </Badge>
                              ) : (
                                <span className='text-muted-foreground'>
                                  {t('Any')}
                                </span>
                              )}
                            </TableCell>
                            <TableCell>
                              {formatWeekdays(rule.weekdays, t('Daily'), t)}
                            </TableCell>
                            <TableCell>
                              {rule.start_time && rule.end_time ? (
                                `${rule.start_time} - ${rule.end_time}`
                              ) : (
                                <span className='text-muted-foreground'>
                                  {t('Any')}
                                </span>
                              )}
                            </TableCell>
                            <TableCell>
                              <Badge variant={ratioVariant(rule.ratio)}>
                                {formatRatio(rule.ratio)}
                              </Badge>
                            </TableCell>
                            <TableCell>{rule.priority ?? 0}</TableCell>
                            <TableCell>
                              <div className='flex justify-end gap-1'>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  disabled={
                                    !canEdit ||
                                    reorderMutation.isPending ||
                                    index === 0
                                  }
                                  onClick={() => handleMove(index, -1)}
                                >
                                  <ArrowUp />
                                  <span className='sr-only'>
                                    {t('Move up')}
                                  </span>
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  disabled={
                                    !canEdit ||
                                    reorderMutation.isPending ||
                                    index === rules.length - 1
                                  }
                                  onClick={() => handleMove(index, 1)}
                                >
                                  <ArrowDown />
                                  <span className='sr-only'>
                                    {t('Move down')}
                                  </span>
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='ghost'
                                  disabled={!canEdit}
                                  onClick={() => openEditDialog(rule)}
                                >
                                  <Pencil />
                                  <span className='sr-only'>{t('Edit')}</span>
                                </Button>
                                <Button
                                  size='icon-sm'
                                  variant='destructive'
                                  disabled={!canEdit}
                                  onClick={() => setDeleteTarget(rule)}
                                >
                                  <Trash2 />
                                  <span className='sr-only'>{t('Delete')}</span>
                                </Button>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {editingRule ? t('Edit rule') : t('New rule')}
            </DialogTitle>
            <DialogDescription className='sr-only'>
              {t('Configure dynamic ratio rule')}
            </DialogDescription>
          </DialogHeader>

          <FieldGroup>
            <Field>
              <FieldLabel htmlFor='dynamic-ratio-group'>
                {t('Group')}
              </FieldLabel>
              <Select
                value={form.group || null}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    group: value ?? '',
                  }))
                }
              >
                <SelectTrigger id='dynamic-ratio-group' className='w-full'>
                  <SelectValue placeholder={t('Select a group')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {groups.map((group) => (
                      <SelectItem key={group} value={group}>
                        {group}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor='dynamic-ratio-models'>
                {t('Models')}
              </FieldLabel>
              <Input
                id='dynamic-ratio-models'
                value={form.models}
                placeholder='gpt-4*, *-preview'
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    models: event.target.value,
                  }))
                }
              />
              <FieldDescription>
                {t('Comma-separated, empty means all models')}
              </FieldDescription>
            </Field>

            <div className='grid gap-3 sm:grid-cols-2'>
              <Field>
                <FieldLabel htmlFor='dynamic-ratio-concurrency'>
                  {t('Concurrency threshold')}
                </FieldLabel>
                <Input
                  id='dynamic-ratio-concurrency'
                  type='number'
                  min='1'
                  value={form.concurrency}
                  placeholder={t('Any')}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      concurrency: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='dynamic-ratio-ratio'>
                  {t('Ratio')}
                </FieldLabel>
                <Input
                  id='dynamic-ratio-ratio'
                  type='number'
                  min='0.01'
                  step='0.1'
                  value={form.ratio}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      ratio: event.target.value,
                    }))
                  }
                />
              </Field>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <Field>
                <FieldLabel htmlFor='dynamic-ratio-start-time'>
                  {t('Start Time')}
                </FieldLabel>
                <Input
                  id='dynamic-ratio-start-time'
                  type='time'
                  value={form.start_time}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      start_time: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='dynamic-ratio-end-time'>
                  {t('End Time')}
                </FieldLabel>
                <Input
                  id='dynamic-ratio-end-time'
                  type='time'
                  value={form.end_time}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      end_time: event.target.value,
                    }))
                  }
                />
              </Field>
            </div>

            <FieldSet>
              <FieldLegend variant='label'>{t('Weekdays')}</FieldLegend>
              <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
                {WEEKDAYS.map((weekday) => {
                  const checked = form.weekdays.includes(weekday.value)
                  return (
                    <FieldLabel
                      key={weekday.value}
                      className={cn(
                        'rounded-lg px-2.5 py-2',
                        checked && 'border-primary/30 bg-primary/5'
                      )}
                    >
                      <Field orientation='horizontal'>
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(value) =>
                            toggleWeekday(weekday.value, value === true)
                          }
                        />
                        <span>{t(weekday.label)}</span>
                      </Field>
                    </FieldLabel>
                  )
                })}
              </div>
            </FieldSet>

            <div className='grid gap-3 sm:grid-cols-2'>
              <Field>
                <FieldLabel htmlFor='dynamic-ratio-priority'>
                  {t('Priority')}
                </FieldLabel>
                <Input
                  id='dynamic-ratio-priority'
                  type='number'
                  value={form.priority}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      priority: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field orientation='horizontal' className='rounded-lg border p-3'>
                <div className='flex flex-1 flex-col gap-0.5'>
                  <FieldLabel>{t('Enabled')}</FieldLabel>
                  <FieldDescription>
                    {t('Enable this rule after saving')}
                  </FieldDescription>
                </div>
                <Switch
                  checked={form.enable}
                  onCheckedChange={(checked) =>
                    setForm((current) => ({ ...current, enable: checked }))
                  }
                />
              </Field>
            </div>
          </FieldGroup>

          <DialogFooter>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={saveRuleMutation.isPending}
            >
              {editingRule ? t('Save Changes') : t('Create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteTarget != null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete rule')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('This dynamic ratio rule will be deleted.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.id)
                }
              }}
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
