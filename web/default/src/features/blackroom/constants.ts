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
import type { TFunction } from 'i18next'

export const BLACKROOM_STATUS_VALUES = [
  'active',
  'released',
  'expired',
] as const

export const BLACKROOM_SOURCE_VALUES = ['auto', 'manual'] as const

export const BLACKROOM_STATUSES: Record<
  string,
  { labelKey: string; variant: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  active: { labelKey: 'Active', variant: 'danger' },
  released: { labelKey: 'Released', variant: 'success' },
  expired: { labelKey: 'Expired', variant: 'warning' },
}

export const BLACKROOM_SOURCES: Record<
  string,
  { labelKey: string; variant: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  auto: { labelKey: 'Auto', variant: 'warning' },
  manual: { labelKey: 'Manual', variant: 'neutral' },
}

export function getBlackroomStatusOptions(t: TFunction) {
  return BLACKROOM_STATUS_VALUES.map((value) => ({
    label: t(BLACKROOM_STATUSES[value].labelKey),
    value,
  }))
}

export function getBlackroomSourceOptions(t: TFunction) {
  return BLACKROOM_SOURCE_VALUES.map((value) => ({
    label: t(BLACKROOM_SOURCES[value].labelKey),
    value,
  }))
}

export function normalizeBlackroomStatus(value: unknown): string {
  const normalized = String(value ?? 'active').toLowerCase()
  return normalized
}

export function normalizeBlackroomSource(value: unknown): string {
  const normalized = String(value ?? 'auto').toLowerCase()
  return normalized
}
