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
import { type TFunction } from 'i18next'
import { type StatusBadgeProps } from '@/components/status-badge'

export const INVITE_CODE_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  USED: 3,
} as const

export const INVITE_CODE_STATUS_VALUES = Object.values(INVITE_CODE_STATUS).map(
  (value) => String(value)
) as `${number}`[]

export const INVITE_CODE_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant'> & {
    labelKey: string
    value: number
  }
> = {
  [INVITE_CODE_STATUS.ENABLED]: {
    labelKey: 'Unused',
    variant: 'success',
    value: INVITE_CODE_STATUS.ENABLED,
  },
  [INVITE_CODE_STATUS.DISABLED]: {
    labelKey: 'Disabled',
    variant: 'neutral',
    value: INVITE_CODE_STATUS.DISABLED,
  },
  [INVITE_CODE_STATUS.USED]: {
    labelKey: 'Used',
    variant: 'neutral',
    value: INVITE_CODE_STATUS.USED,
  },
} as const

export const INVITE_CODE_FILTER_EXPIRED = 'expired'

export function getInviteCodeStatusOptions(t: TFunction) {
  return [
    ...Object.values(INVITE_CODE_STATUSES).map((config) => ({
      label: t(config.labelKey),
      value: String(config.value),
    })),
    {
      label: t('Expired'),
      value: INVITE_CODE_FILTER_EXPIRED,
    },
  ]
}

export const INVITE_CODE_VALIDATION = {
  NAME_MIN_LENGTH: 1,
  NAME_MAX_LENGTH: 20,
  COUNT_MIN: 1,
  COUNT_MAX: 100000,
  KEY_PREFIX_MAX_LENGTH: 24,
} as const

export const INVITE_CODE_SUCCESS_MESSAGES = {
  CREATED: 'Invite code(s) created successfully',
  UPDATED: 'Invite code updated successfully',
  DELETED: 'Invite code deleted successfully',
  ENABLED: 'Invite code enabled successfully',
  DISABLED: 'Invite code disabled successfully',
} as const
