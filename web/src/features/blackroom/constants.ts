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

export const BLACKROOM_TAB_VALUES = ['bans', 'ip-audit'] as const

export const BLACKROOM_DEFAULT_TAB = 'bans'

export const BLACKROOM_TAB_IP_AUDIT = 'ip-audit'

/** IP 审计默认每页条数。 */
export const BLACKROOM_IP_AUDIT_DEFAULT_PAGE_SIZE = 20

/** 后端 `/api/blackroom/ip-audit` 接受的每页条数上限。 */
export const BLACKROOM_IP_AUDIT_MAX_PAGE_SIZE = 100

export const BLACKROOM_STATUS_VALUES = [
  'active',
  'released',
  'expired',
] as const

export const BLACKROOM_SOURCE_VALUES = ['auto', 'manual', 'external'] as const

export const BLACKROOM_STATUSES: Record<
  string,
  { labelKey: string; variant: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  active: { labelKey: 'Active', variant: 'danger' },
  released: { labelKey: 'Ban Released', variant: 'success' },
  expired: { labelKey: 'Expired', variant: 'warning' },
}

export const BLACKROOM_SOURCES: Record<
  string,
  { labelKey: string; variant: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  auto: { labelKey: 'Auto', variant: 'warning' },
  manual: { labelKey: 'Manual', variant: 'neutral' },
  external: { labelKey: 'External', variant: 'danger' },
}

const BLACKROOM_BAN_EVENT_TYPES: Record<
  string,
  { labelKey: string; variant: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  apply: { labelKey: 'Ban applied', variant: 'danger' },
  reapply: { labelKey: 'Ban reapplied', variant: 'danger' },
  extend: { labelKey: 'Ban extended', variant: 'warning' },
  release: { labelKey: 'Ban Released', variant: 'success' },
  expire: { labelKey: 'Ban expired', variant: 'neutral' },
  shadow_match: { labelKey: 'Shadow match', variant: 'warning' },
}

function normalizeBlackroomBanEventType(value: unknown): string {
  return String(value ?? '').toLowerCase()
}

export function getBlackroomBanEventConfig(value: unknown): {
  labelKey: string
  variant: 'success' | 'warning' | 'danger' | 'neutral'
} {
  const eventType = normalizeBlackroomBanEventType(value)
  if (!Object.hasOwn(BLACKROOM_BAN_EVENT_TYPES, eventType)) {
    return { labelKey: eventType || '-', variant: 'neutral' }
  }
  return BLACKROOM_BAN_EVENT_TYPES[eventType]
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

// 后台任务把到期记录标记为 expired 前有最多一个检查周期的延迟，
// 展示层直接按 banned_until 判定，避免"已到期仍显示生效"。
export function resolveBlackroomDisplayStatus(
  entry: { status?: unknown; banned_until?: number | null },
  nowSeconds: number = Math.floor(Date.now() / 1000)
): string {
  const status = normalizeBlackroomStatus(entry.status)
  const bannedUntil = entry.banned_until ?? 0
  if (status === 'active' && bannedUntil > 0 && bannedUntil <= nowSeconds) {
    return 'expired'
  }
  return status
}

export function normalizeBlackroomSource(value: unknown): string {
  const normalized = String(value ?? 'auto').toLowerCase()
  return normalized
}
