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
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { saveGroupMultiplier } from './api'
import {
  groupPolicySchema,
  multiplierModeLabel,
  type GroupMultiplierPolicy,
  type GroupMultiplierStatus,
  type MultiplierMode,
} from './group-policy'

export function GroupPolicyDialog(props: {
  status: GroupMultiplierStatus
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<GroupMultiplierPolicy>({
    resolver: zodResolver(groupPolicySchema(t)),
    defaultValues: props.status.policy,
  })
  const tiers = useFieldArray({ control: form.control, name: 'tiers' })
  const mode = form.watch('mode')
  const mutation = useMutation({
    mutationFn: (policy: GroupMultiplierPolicy) =>
      saveGroupMultiplier(props.status.group, policy, props.status.version),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['group-multipliers'] })
      toast.success(t('Saved successfully'))
      props.onClose()
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
      title={`${t('Multiplier mode')} · ${props.status.group}`}
      description={t(
        'Only the selected mode applies. Other rules are kept for switching back.'
      )}
      footer={
        <>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='group-multiplier-policy'
            disabled={mutation.isPending}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='group-multiplier-policy'
          onSubmit={form.handleSubmit((policy) => mutation.mutate(policy))}
          className='space-y-5'
        >
          <FormField
            control={form.control}
            name='mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Multiplier mode')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(value) => {
                    if (!value) return
                    field.onChange(value)
                    if (value === 'concurrency' && tiers.fields.length === 0) {
                      tiers.append({ minimum: 0, multiplier: 1 })
                    }
                  }}
                  disabled={mutation.isPending}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue>
                        {multiplierModeLabel(field.value, t)}
                      </SelectValue>
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {(
                      ['fixed', 'balance', 'concurrency'] as MultiplierMode[]
                    ).map((item) => (
                      <SelectItem key={item} value={item}>
                        {multiplierModeLabel(item, t)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
          {mode === 'concurrency' && (
            <fieldset className='space-y-4' disabled={mutation.isPending}>
              <legend className='mb-2 font-medium'>
                {t('Concurrency tiers')}
              </legend>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'The admitted request is included in concurrency. Its multiplier stays locked until completion.'
                )}
              </p>
              {tiers.fields.map((field, index) => (
                <div
                  key={field.id}
                  className='grid grid-cols-[1fr_1fr_auto] items-start gap-3'
                >
                  <Field>
                    <FieldLabel htmlFor={`${field.id}-minimum`}>
                      {t('Concurrency threshold')} {index + 1}
                    </FieldLabel>
                    <Input
                      id={`${field.id}-minimum`}
                      {...form.register(`tiers.${index}.minimum`, {
                        valueAsNumber: true,
                      })}
                      type='number'
                      min={0}
                      max={1000000}
                      step={1}
                      readOnly={index === 0}
                      aria-invalid={Boolean(
                        form.formState.errors.tiers?.[index]?.minimum
                      )}
                    />
                    <FieldError
                      errors={[form.formState.errors.tiers?.[index]?.minimum]}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${field.id}-multiplier`}>
                      {t('Dynamic multiplier')} {index + 1}
                    </FieldLabel>
                    <Input
                      id={`${field.id}-multiplier`}
                      {...form.register(`tiers.${index}.multiplier`, {
                        valueAsNumber: true,
                      })}
                      type='number'
                      min={0.000001}
                      max={1000}
                      step='any'
                      aria-invalid={Boolean(
                        form.formState.errors.tiers?.[index]?.multiplier
                      )}
                    />
                    <FieldError
                      errors={[
                        form.formState.errors.tiers?.[index]?.multiplier,
                      ]}
                    />
                  </Field>
                  <Button
                    type='button'
                    variant='ghost'
                    className='mt-6'
                    disabled={index === 0 || mutation.isPending}
                    onClick={() => tiers.remove(index)}
                    aria-label={`${t('Remove tier')} ${index + 1}`}
                  >
                    {t('Remove')}
                  </Button>
                </div>
              ))}
              {form.formState.errors.tiers?.root?.message && (
                <p role='alert' className='text-destructive text-sm'>
                  {form.formState.errors.tiers.root.message}
                </p>
              )}
              <Button
                type='button'
                variant='outline'
                disabled={tiers.fields.length >= 32 || mutation.isPending}
                onClick={() => {
                  const last = form.getValues('tiers').at(-1)
                  tiers.append({
                    minimum: (last?.minimum ?? 0) + 1,
                    multiplier: last?.multiplier ?? 1,
                  })
                }}
              >
                {t('Add tier')}
              </Button>
            </fieldset>
          )}
          {mode === 'balance' && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Existing balance, model, and time rules apply. Concurrency tiers are inactive.'
              )}
            </p>
          )}
        </form>
      </Form>
    </Dialog>
  )
}
