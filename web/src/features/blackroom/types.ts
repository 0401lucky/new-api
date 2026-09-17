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

export const blackroomEntrySchema = z
  .object({
    id: z.number(),
    user_id: z.number(),
    username: z.string().optional().nullable(),
    status: z.string().optional().nullable(),
    source: z.string().optional().nullable(),
    reason: z.string().optional().nullable(),
    evidence: z.string().optional().nullable(),
    ip_count: z.number().optional().nullable(),
    ip_list: z.string().optional().nullable(),
    window_start: z.number().optional().nullable(),
    window_end: z.number().optional().nullable(),
    ban_duration_seconds: z.number().optional().nullable(),
    banned_until: z.number().optional().nullable(),
    created_at: z.number().optional().nullable(),
    updated_at: z.number().optional().nullable(),
    released_at: z.number().optional().nullable(),
    released_by: z.number().optional().nullable(),
    release_reason: z.string().optional().nullable(),
  })
  .passthrough()

export type BlackroomEntry = z.infer<typeof blackroomEntrySchema>

export type BlackroomStatus = 'active' | 'released' | 'expired'
export type BlackroomSource = 'auto' | 'manual' | 'external'

export interface BlackroomRule {
  ip_count: number
  duration_hours: number
  permanent: boolean
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface PageInfo<T> {
  items: T[]
  total: number
  page?: number
  page_size?: number
}

export interface GetBlackroomParams {
  p?: number
  page_size?: number
  filter?: string
  status?: string
  source?: string
  user_id?: number
}

export interface BlackroomSetting {
  enabled: boolean
  auto_ban_enabled: boolean
  lookback_hours: number
  check_interval_minutes: number
  min_requests: number
  rules: BlackroomRule[]
  escalation_window_days: number
  escalation_temporary_ban_count: number
  exempt_user_ids: number[]
  exempt_groups: string[]
  shadow_mode: boolean
  realtime_enabled: boolean
  geo_enabled: boolean
  geo_country_count: number
  geo_asn_count: number
  geo_min_gap_seconds: number
  geo_duration_hours: number
  country_mmdb_path: string
  asn_mmdb_path: string
}

export interface ManualBanPayload {
  user_id: number
  duration_hours: number
  permanent: boolean
  reason: string
}

export interface ReleasePayload {
  reason: string
}

/** MMDB 解析器就绪详情，对应 `/api/blackroom/status` 的 `resolver`。 */
export interface BlackroomGeoResolverStatus {
  ready: boolean
  country_ready: boolean
  asn_ready: boolean
  version: string
  /** `not_configured` | `open_failed` | `not_initialized` */
  error_code?: string
}

/**
 * 判定链路的就绪汇总。`blocking` 里的每一项都代表自动封禁被挡住的
 * 原因，取值：`blackroom_disabled` | `auto_ban_disabled` |
 * `geo_resolver_not_ready`。
 */
export interface BlackroomStatusSummary {
  enabled: boolean
  auto_ban_enabled: boolean
  shadow_mode: boolean
  realtime_enabled: boolean
  geo_enabled: boolean
  geo_effective: boolean
  resolver: BlackroomGeoResolverStatus
  blocking: string[]
}

/** IP 视角下的关联用户。 */
export interface BlackroomIPAuditUser {
  user_id: number
  username: string
  request_count: number
}

/** IP 维度的一行聚合结果。 */
export interface BlackroomIPAuditItem {
  ip: string
  request_count: number
  user_count: number
  first_seen_at: number
  last_seen_at: number
  users: BlackroomIPAuditUser[]
  /** true 表示只返回了前 5 个关联用户。 */
  users_truncated: boolean
}

export interface BlackroomIPAuditResult {
  start_at: number
  end_at: number
  total: number
  page: number
  page_size: number
  items: BlackroomIPAuditItem[]
}

/**
 * 一条只追加的封禁事件。状态表上被覆盖掉的中间状态在这里仍然可追溯，
 * `ban_id` 为 0 表示该事件没有对应封禁（目前只有影子模式命中）。
 */
export interface BlackroomBanEvent {
  id: number
  ban_id: number
  user_id: number
  /** `apply` | `reapply` | `extend` | `release` | `expire` | `shadow_match` */
  event_type: string
  source: string
  reason: string
  /** JSON 字符串，展示时格式化或折叠。 */
  evidence: string
  ip_count: number
  /** JSON 数组字符串。 */
  ip_list: string
  window_start: number
  window_end: number
  ban_duration_seconds: number
  banned_until: number
  /** 手动操作时的操作人，0 表示系统。 */
  actor_user_id: number
  created_at: number
}

/**
 * IP 审计视图用到的 URL 查询参数。合法取值由路由的 `validateSearch`
 * 保证，这里只描述组件与 Hook 需要消费的字段。
 */
export interface BlackroomIPAuditSearchParams {
  ipPage?: number
  ipPageSize?: number
  ipFilter?: string
  /** Unix 秒；0 与缺省同义，表示不设下界。 */
  ipStart?: number
  /** Unix 秒；0 与缺省同义，表示不设上界。 */
  ipEnd?: number
}

export interface GetBlackroomIPAuditParams {
  p?: number
  page_size?: number
  filter?: string
  start_at?: number
  end_at?: number
}

export interface GetBlackroomEventsParams {
  user_id: number
  limit?: number
}

export type BlackroomDialogType =
  | 'manual-ban'
  | 'setting'
  | 'release'
  | 'events'
