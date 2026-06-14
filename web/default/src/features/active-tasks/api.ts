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
import type {
  ActiveTaskRankResponse,
  ActiveTaskStats,
  ApiResponse,
  HighActiveTaskHistoryResponse,
  UserTokenUsageResponse,
} from './types'

export async function getActiveTaskRank(params: {
  window?: number
  limit?: number
}): Promise<ApiResponse<ActiveTaskRankResponse>> {
  const res = await api.get('/api/active_task/rank', { params })
  return res.data
}

export async function getActiveTaskStats(): Promise<
  ApiResponse<ActiveTaskStats>
> {
  const res = await api.get('/api/active_task/stats')
  return res.data
}

export async function getHighActiveTaskHistory(params: {
  limit?: number
  user_id?: number
}): Promise<ApiResponse<HighActiveTaskHistoryResponse>> {
  const res = await api.get('/api/active_task/history', { params })
  return res.data
}

export async function getUserTokenUsage24h(
  userId: number
): Promise<ApiResponse<UserTokenUsageResponse>> {
  const res = await api.get('/api/active_task/user_token_usage', {
    params: { user_id: userId },
  })
  return res.data
}
