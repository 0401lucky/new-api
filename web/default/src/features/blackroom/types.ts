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
export type BlackroomSource = 'auto' | 'manual'

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

export type BlackroomDialogType = 'manual-ban' | 'setting' | 'release'
