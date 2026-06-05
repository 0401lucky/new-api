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
