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
  GetInviteCodesParams,
  GetInviteCodesResponse,
  InviteCode,
  InviteCodeFormData,
  SearchInviteCodesParams,
} from './types'

export async function getInviteCodes(
  params: GetInviteCodesParams = {}
): Promise<GetInviteCodesResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/invitation/?p=${p}&page_size=${page_size}`)
  return res.data
}

export async function searchInviteCodes(
  params: SearchInviteCodesParams
): Promise<GetInviteCodesResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get('/api/invitation/search', {
    params: { keyword, p, page_size },
  })
  return res.data
}

export async function getInviteCode(
  id: number
): Promise<ApiResponse<InviteCode>> {
  const res = await api.get(`/api/invitation/${id}`)
  return res.data
}

export async function createInviteCode(
  data: InviteCodeFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/invitation/', data)
  return res.data
}

export async function updateInviteCode(
  data: InviteCodeFormData & { id: number }
): Promise<ApiResponse<InviteCode>> {
  const res = await api.put('/api/invitation/', data)
  return res.data
}

export async function updateInviteCodeStatus(
  id: number,
  status: number
): Promise<ApiResponse<InviteCode>> {
  const res = await api.put('/api/invitation/?status_only=true', {
    id,
    status,
  })
  return res.data
}

export async function deleteInviteCode(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/invitation/${id}/`)
  return res.data
}

export async function deleteInvalidInviteCodes(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/invitation/invalid')
  return res.data
}

export async function deleteValidInviteCodes(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/invitation/valid')
  return res.data
}
