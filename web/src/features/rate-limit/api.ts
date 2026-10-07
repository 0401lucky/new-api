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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export interface RateLimitMetric {
  used: number | null
  limit: number
}

export interface RateLimitUsageData {
  enabled: boolean
  exempt: boolean
  window_minutes: number
  total_mode: 'sliding_window' | 'token_bucket'
  groups: {
    group: string
    total: RateLimitMetric
    success: RateLimitMetric
    concurrency: RateLimitMetric
  }[]
}

export async function getRateLimitUsage(
  signal?: AbortSignal
): Promise<RateLimitUsageData> {
  const response = await api.get<{
    success: boolean
    data: RateLimitUsageData
  }>('/api/user/self/rate_limit', { signal })
  return requireServerSuccess(response.data).data
}
