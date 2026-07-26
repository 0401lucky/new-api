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
  DuplicateFingerprint,
  FingerprintPageParams,
  FingerprintRecord,
  FingerprintSearchParams,
  PageResponse,
} from './types'

function withPageParams(params: FingerprintPageParams = {}) {
  return {
    p: params.p ?? 1,
    page_size: params.page_size ?? 20,
  }
}

export async function getDuplicateFingerprints(
  params: FingerprintPageParams = {}
): Promise<ApiResponse<PageResponse<DuplicateFingerprint>>> {
  const res = await api.get('/api/fingerprint/duplicates', {
    params: withPageParams(params),
  })
  return res.data
}

export async function getFingerprints(
  params: FingerprintPageParams = {}
): Promise<ApiResponse<PageResponse<FingerprintRecord>>> {
  const res = await api.get('/api/fingerprint/', {
    params: withPageParams(params),
  })
  return res.data
}

export async function searchFingerprints(
  params: FingerprintSearchParams
): Promise<ApiResponse<PageResponse<FingerprintRecord>>> {
  const res = await api.get('/api/fingerprint/search', {
    params: {
      ...withPageParams(params),
      keyword: params.keyword ?? '',
    },
  })
  return res.data
}

export async function findUsersByVisitorId(params: {
  visitorId: string
  ip?: string
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageResponse<FingerprintRecord>>> {
  const res = await api.get('/api/fingerprint/users', {
    params: {
      visitor_id: params.visitorId,
      ip: params.ip || undefined,
      ...withPageParams(params),
    },
  })
  return res.data
}

export async function findUsersByIP(params: {
  ip: string
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageResponse<FingerprintRecord>>> {
  const res = await api.get('/api/fingerprint/users_by_ip', {
    params: {
      ip: params.ip,
      ...withPageParams(params),
    },
  })
  return res.data
}
