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
  ApiResponse,
  BlackroomBanEvent,
  BlackroomEntry,
  BlackroomIPAuditResult,
  BlackroomSetting,
  BlackroomStatusSummary,
  GetBlackroomEventsParams,
  GetBlackroomIPAuditParams,
  GetBlackroomParams,
  ManualBanPayload,
  PageInfo,
  ReleasePayload,
} from './types'

export async function getBlackroom(
  params: GetBlackroomParams = {}
): Promise<ApiResponse<PageInfo<BlackroomEntry>>> {
  const queryParams = new URLSearchParams()
  queryParams.set('p', String(params.p ?? 1))
  queryParams.set('page_size', String(params.page_size ?? 20))
  if (params.filter) queryParams.set('filter', params.filter)
  if (params.status) queryParams.set('status', params.status)
  if (params.source) queryParams.set('source', params.source)
  if (params.user_id) queryParams.set('user_id', String(params.user_id))

  const res = await api.get(`/api/blackroom?${queryParams.toString()}`)
  return res.data
}

export async function getBlackroomSetting(): Promise<
  ApiResponse<BlackroomSetting>
> {
  const res = await api.get('/api/blackroom/setting')
  return res.data
}

export async function updateBlackroomSetting(
  data: BlackroomSetting
): Promise<ApiResponse<BlackroomSetting>> {
  const res = await api.put('/api/blackroom/setting', data)
  return res.data
}

export async function getBlackroomStatus(): Promise<
  ApiResponse<BlackroomStatusSummary>
> {
  const res = await api.get('/api/blackroom/status')
  return res.data
}

export async function getBlackroomIPAudit(
  params: GetBlackroomIPAuditParams = {}
): Promise<ApiResponse<BlackroomIPAuditResult>> {
  const queryParams = new URLSearchParams()
  queryParams.set('p', String(params.p ?? 1))
  queryParams.set('page_size', String(params.page_size ?? 20))
  if (params.filter) queryParams.set('filter', params.filter)
  if (params.start_at) queryParams.set('start_at', String(params.start_at))
  if (params.end_at) queryParams.set('end_at', String(params.end_at))

  const res = await api.get(`/api/blackroom/ip-audit?${queryParams.toString()}`)
  return res.data
}

export async function getBlackroomEvents(
  params: GetBlackroomEventsParams
): Promise<ApiResponse<BlackroomBanEvent[]>> {
  const queryParams = new URLSearchParams()
  queryParams.set('user_id', String(params.user_id))
  if (params.limit) queryParams.set('limit', String(params.limit))

  const res = await api.get(`/api/blackroom/events?${queryParams.toString()}`)
  return res.data
}

export async function manualBanUser(
  data: ManualBanPayload
): Promise<ApiResponse> {
  const res = await api.post('/api/blackroom/manual-ban', data)
  return res.data
}

export async function releaseBlackroomEntry(
  id: number,
  data: ReleasePayload
): Promise<ApiResponse> {
  const res = await api.post(`/api/blackroom/${id}/release`, data)
  return res.data
}

export async function scanBlackroom(): Promise<ApiResponse> {
  const res = await api.post('/api/blackroom/scan')
  return res.data
}
