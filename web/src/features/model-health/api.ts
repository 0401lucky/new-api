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
  ApiEnvelope,
  ModelHealthHourlyStat,
  PublicModelHealthPayload,
} from './types'

export async function getPublicModelHealthLast24h() {
  const res = await api.get<ApiEnvelope<PublicModelHealthPayload>>(
    '/api/public/model_health/hourly_last24h',
    { skipErrorHandler: true, skipBusinessError: true }
  )
  return res.data
}

export async function getModelHealthHourly(params: {
  model_name: string
  start_hour: number
  end_hour: number
}) {
  const res = await api.get<ApiEnvelope<ModelHealthHourlyStat[]>>(
    '/api/model_health/hourly',
    { params, skipBusinessError: true }
  )
  return res.data
}

export async function getEnabledModelNames() {
  const res = await api.get<ApiEnvelope<unknown>>(
    '/api/channel/models_enabled',
    {
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}
