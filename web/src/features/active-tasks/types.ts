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

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface ActiveTaskRankItem {
  user_id: number
  username: string
  active_slots: number
}

export interface ActiveTaskRankResponse {
  rank: ActiveTaskRankItem[]
  window_seconds: number
}

export interface ActiveTaskStats {
  total_slots: number
  active_slots: number
  max_global_slots: number
  max_user_slots: number
  active_users: number
  window_seconds: number
}

export interface HighActiveTaskRecord {
  id: number
  user_id: number
  username: string
  active_slots: number
  window_secs: number
  created_at: number
}

export interface HighActiveTaskHistoryResponse {
  records: HighActiveTaskRecord[]
  total: number
}

export interface ModelTokenUsage {
  model_name: string
  total_tokens: number
  request_count: number
}

export interface UserTokenUsageResponse {
  user_id: number
  start_timestamp: number
  end_timestamp: number
  models: ModelTokenUsage[]
  total_tokens: number
  total_requests: number
}
