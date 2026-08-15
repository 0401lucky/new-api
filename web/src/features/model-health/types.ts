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

export type ModelHealthHourlyStat = {
  model_name: string
  hour_start_ts: number
  success_slices: number
  total_slices: number
  success_rate: number
  total_requests: number
  error_requests: number
  success_requests: number
  qualified_success_requests: number
  success_tokens: number
}

export type PublicModelHealthHourlyStat = ModelHealthHourlyStat & {
  is_filled?: boolean
}

export type PublicModelHealthPayload = {
  start_hour: number
  end_hour: number
  rows: PublicModelHealthHourlyStat[]
}

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data: T
}

export type ModelHealthStatus =
  | 'operational'
  | 'degraded'
  | 'outage'
  | 'no_data'

export type ModelHealthGlobalStatus = 'operational' | 'degraded' | 'outage'

export type ModelHealthPeriod = '7d' | '15d' | '30d'

export type ModelHealthOverviewTimelineItem = {
  hour_start_ts: number
  success_rate: number
  total_requests: number
  error_requests: number
  success_tokens: number
}

export type ModelHealthOverviewModel = {
  model_name: string
  status: ModelHealthStatus
  availability: number | null
  availability_success: number
  availability_total: number
  avg_latency_ms: number | null
  avg_ttft_ms: number | null
  success_tokens_24h: number
  timeline: ModelHealthOverviewTimelineItem[]
}

export type ModelHealthOverviewStats = {
  total_models: number
  healthy_models: number
  overall_rate_24h: number
  total_tokens_24h: number
}

export type ModelHealthOverviewPayload = {
  updated_at: number
  period: ModelHealthPeriod
  global_status: ModelHealthGlobalStatus
  stats: ModelHealthOverviewStats
  models: ModelHealthOverviewModel[]
}
