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

export interface RecentCallBody {
  body_type?: string
  body?: string
  truncated?: boolean
  omitted?: boolean
  omit_reason?: string
}

export interface RecentCallRequest extends RecentCallBody {
  method: string
  path: string
  headers?: Record<string, string>
}

export interface RecentCallResponse extends RecentCallBody {
  status_code: number
  headers?: Record<string, string>
}

export interface RecentCallStream {
  chunks?: string[]
  chunks_truncated?: boolean
  aggregated_text?: string
  aggregated_truncated?: boolean
}

export interface RecentCallErrorInfo {
  message?: string
  type?: string
  code?: string
  status?: number
}

export interface RecentCallPromptCheckMatch {
  name?: string
  weight?: number
  category?: string
  strict?: boolean
  matched?: string
}

export interface RecentCallPromptCheckInfo {
  action?: string
  mode?: string
  score?: number
  raw_score?: number
  threshold?: number
  strict_threshold?: number
  strict_hit?: boolean
  matches?: RecentCallPromptCheckMatch[]
  reason?: string
  preview?: string
  full_text?: string
  extracted_chars?: number
  reviewed?: boolean
  review_flagged?: boolean
  review_model?: string
  review_error?: string
}

export interface RecentCallRecord {
  id: number
  created_at: string
  user_id: number
  username?: string
  channel_id?: number
  model_name?: string
  method: string
  path: string
  request: RecentCallRequest
  response?: RecentCallResponse
  stream?: RecentCallStream
  error?: RecentCallErrorInfo
  prompt_check?: RecentCallPromptCheckInfo
}

export interface RecentCallsListResponse {
  data: RecentCallRecord[]
  limit: number
}

export interface RecentCallDetailResponse {
  data: RecentCallRecord
}
