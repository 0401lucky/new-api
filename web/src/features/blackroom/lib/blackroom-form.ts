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
import { z } from 'zod'
import type { TFunction } from 'i18next'
import type { BlackroomRule, BlackroomSetting } from '../types'

export function getManualBanFormSchema(t: TFunction) {
  return z
    .object({
      user_id: z.coerce
        .number()
        .int()
        .positive(t('Please enter a valid user ID')),
      duration_hours: z.coerce
        .number()
        .int()
        .min(0, t('Duration must be zero or greater')),
      permanent: z.boolean(),
      reason: z.string().trim().min(1, t('Please enter a reason')),
    })
    .refine((value) => value.permanent || value.duration_hours > 0, {
      path: ['duration_hours'],
      message: t('Duration is required for temporary bans'),
    })
}

export function getBlackroomSettingFormSchema(t: TFunction) {
  return z
    .object({
      enabled: z.boolean(),
      auto_ban_enabled: z.boolean(),
      lookback_hours: z.coerce
        .number()
        .int()
        .min(1, t('Lookback hours must be at least 1')),
      check_interval_minutes: z.coerce
        .number()
        .int()
        .min(1, t('Scan interval must be at least 1 minute')),
      min_requests: z.coerce
        .number()
        .int()
        .min(0, t('Minimum requests must be zero or greater')),
      rules_text: z.string().trim().min(1, t('Rules are required')),
      escalation_window_days: z.coerce
        .number()
        .int()
        .min(1, t('Escalation window must be at least 1 day')),
      escalation_temporary_ban_count: z.coerce
        .number()
        .int()
        .min(0, t('Escalation count must be zero or greater')),
      exempt_user_ids_text: z.string(),
      exempt_groups_text: z.string(),
    })
    .refine((value) => parseRulesText(value.rules_text).length > 0, {
      path: ['rules_text'],
      message: t('Rules must be a valid JSON array'),
    })
}

export type ManualBanFormValues = z.infer<
  ReturnType<typeof getManualBanFormSchema>
>

export type BlackroomSettingFormValues = z.infer<
  ReturnType<typeof getBlackroomSettingFormSchema>
>

export const MANUAL_BAN_FORM_DEFAULT_VALUES: ManualBanFormValues = {
  user_id: 0,
  duration_hours: 24,
  permanent: false,
  reason: '',
}

export const BLACKROOM_SETTING_FORM_DEFAULT_VALUES: BlackroomSettingFormValues =
  {
    enabled: true,
    auto_ban_enabled: false,
    lookback_hours: 24,
    check_interval_minutes: 10,
    min_requests: 0,
    rules_text: formatRulesText([
      { ip_count: 8, duration_hours: 6, permanent: false },
      { ip_count: 13, duration_hours: 72, permanent: false },
      { ip_count: 17, duration_hours: 0, permanent: true },
    ]),
    escalation_window_days: 30,
    escalation_temporary_ban_count: 3,
    exempt_user_ids_text: '',
    exempt_groups_text: '',
  }

function formatRulesText(rules: BlackroomRule[] | undefined): string {
  return JSON.stringify(rules && rules.length > 0 ? rules : [], null, 2)
}

function parseCSVNumbers(value: string): number[] {
  return value
    .split(/[\n,]+/)
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
}

function parseCSVStrings(value: string): string[] {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export function parseRulesText(value: string): BlackroomRule[] {
  try {
    const parsed = JSON.parse(value)
    if (!Array.isArray(parsed)) return []
    return parsed
      .map((item) => ({
        ip_count: Number(item?.ip_count),
        duration_hours: Number(item?.duration_hours ?? 0),
        permanent: Boolean(item?.permanent),
      }))
      .filter(
        (item) =>
          Number.isInteger(item.ip_count) &&
          item.ip_count > 0 &&
          (item.permanent ||
            (Number.isInteger(item.duration_hours) && item.duration_hours > 0))
      )
  } catch {
    return []
  }
}

export function transformFormValuesToSetting(
  values: BlackroomSettingFormValues
): BlackroomSetting {
  return {
    enabled: values.enabled,
    auto_ban_enabled: values.auto_ban_enabled,
    lookback_hours: values.lookback_hours,
    check_interval_minutes: values.check_interval_minutes,
    min_requests: values.min_requests,
    rules: parseRulesText(values.rules_text),
    escalation_window_days: values.escalation_window_days,
    escalation_temporary_ban_count: values.escalation_temporary_ban_count,
    exempt_user_ids: parseCSVNumbers(values.exempt_user_ids_text),
    exempt_groups: parseCSVStrings(values.exempt_groups_text),
  }
}

export function transformSettingToFormDefaults(
  setting?: BlackroomSetting
): BlackroomSettingFormValues {
  return {
    enabled: Boolean(setting?.enabled ?? true),
    auto_ban_enabled: Boolean(setting?.auto_ban_enabled ?? false),
    lookback_hours: Number(setting?.lookback_hours ?? 24),
    check_interval_minutes: Number(setting?.check_interval_minutes ?? 10),
    min_requests: Number(setting?.min_requests ?? 0),
    rules_text: formatRulesText(setting?.rules),
    escalation_window_days: Number(setting?.escalation_window_days ?? 30),
    escalation_temporary_ban_count: Number(
      setting?.escalation_temporary_ban_count ?? 3
    ),
    exempt_user_ids_text: (setting?.exempt_user_ids ?? []).join(','),
    exempt_groups_text: (setting?.exempt_groups ?? []).join(','),
  }
}
