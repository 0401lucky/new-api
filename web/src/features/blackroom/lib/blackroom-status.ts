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
import type { BlackroomStatusSummary } from '../types'

/** 后台返回的拦截原因码 → i18n 键。未知原因码不翻译，直接展示原码。 */
const BLACKROOM_BLOCKING_REASON_KEYS: Record<string, string> = {
  blackroom_disabled: 'Blackroom is disabled, so no ban is applied.',
  auto_ban_disabled: 'Auto ban is disabled, so only manual bans apply.',
  geo_resolver_not_ready:
    'The MMDB resolver is not ready, so geo blocking is skipped.',
}

/** MMDB 解析器不可用的错误码 → i18n 键。 */
const BLACKROOM_RESOLVER_ERROR_KEYS: Record<string, string> = {
  not_configured: 'No MMDB path is configured.',
  open_failed: 'The MMDB file could not be opened.',
  not_initialized: 'The MMDB resolver has not been initialized.',
}

export interface BlackroomGeoReadiness {
  /** 地理判定是否真的在生效（管理员开启且解析器就绪）。 */
  effective: boolean
  /** 未生效时的原因 i18n 键；状态未知时为 null。 */
  reasonKey: string | null
}

export function getBlackroomBlockingReasonKey(reason: string): string | null {
  if (!Object.hasOwn(BLACKROOM_BLOCKING_REASON_KEYS, reason)) return null
  return BLACKROOM_BLOCKING_REASON_KEYS[reason]
}

function getBlackroomResolverErrorKey(
  errorCode: string | undefined
): string | null {
  if (!errorCode) return null
  if (!Object.hasOwn(BLACKROOM_RESOLVER_ERROR_KEYS, errorCode)) return null
  return BLACKROOM_RESOLVER_ERROR_KEYS[errorCode]
}

/**
 * 区分「管理员没开」和「开了但 MMDB 没就绪」——两者的处置动作不同，
 * 后者还需要按错误码进一步区分是没配路径还是文件打不开。
 */
export function resolveBlackroomGeoReadiness(
  status?: BlackroomStatusSummary | null
): BlackroomGeoReadiness {
  if (!status) {
    return { effective: false, reasonKey: null }
  }
  if (status.geo_effective) {
    return { effective: true, reasonKey: null }
  }
  if (!status.geo_enabled) {
    return { effective: false, reasonKey: 'Geo blocking is disabled.' }
  }
  return {
    effective: false,
    reasonKey:
      getBlackroomResolverErrorKey(status.resolver?.error_code) ??
      'The MMDB resolver is not ready.',
  }
}
