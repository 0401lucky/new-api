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
import dayjs from '@/lib/dayjs'

import type { ModelHealthHourlyStat } from './types'

export function floorToHour(tsSec: number) {
  return Math.floor(tsSec / 3600) * 3600
}

export function getDefaultHourRangeLast24h() {
  const nowSec = Math.floor(Date.now() / 1000)
  const endHour = floorToHour(nowSec) + 3600
  const startHour = endHour - 24 * 3600
  return { startHour, endHour }
}

export function getHourRange(hours: number) {
  const nowSec = Math.floor(Date.now() / 1000)
  const endHour = floorToHour(nowSec) + 3600
  const startHour = endHour - hours * 3600
  return { startHour, endHour }
}

export function timestamp2string(tsSec: number) {
  return dayjs(tsSec * 1000).format('YYYY-MM-DD HH:mm:ss')
}

export function formatRate(rate: number) {
  if (!Number.isFinite(rate)) return '0.00%'
  return `${(rate * 100).toFixed(2)}%`
}

export function hourLabel(tsSec?: number) {
  if (!tsSec) return ''
  const full = timestamp2string(tsSec)
  return `${full.slice(11, 13)}:00`
}

export function formatTokens(value: number) {
  const raw = Number(value) || 0
  const n = Math.abs(raw)
  if (!Number.isFinite(n) || n === 0) return '0'

  const sign = raw < 0 ? '-' : ''
  const units = [
    { value: 1_000_000_000, suffix: 'B' },
    { value: 1_000_000, suffix: 'M' },
    { value: 1_000, suffix: 'K' },
  ]
  const unit = units.find((item) => n >= item.value)
  if (!unit) return `${sign}${Math.trunc(n)}`

  const compact = Math.floor((n / unit.value) * 10) / 10
  return `${sign}${compact.toFixed(1).replace(/\.0$/, '')}${unit.suffix}`
}

export function percentileNearestRank(values: number[], p: number) {
  const arr = (values || [])
    .filter((v) => Number.isFinite(v))
    .sort((a, b) => a - b)
  if (arr.length === 0) return 0
  const pp = Math.max(0, Math.min(1, Number(p) || 0))
  const idx = Math.floor((arr.length - 1) * pp)
  return Number(arr[idx]) || 0
}

export function toDateTimeLocalValue(tsSec: number) {
  return dayjs(tsSec * 1000).format('YYYY-MM-DDTHH:mm')
}

export function dateTimeLocalValueToHour(value: string) {
  const ts = Math.floor(new Date(value).getTime() / 1000)
  return floorToHour(ts)
}

export function formatLatencyMs(value: number | null | undefined) {
  if (
    value === null ||
    value === undefined ||
    !Number.isFinite(value) ||
    value <= 0
  ) {
    return '—'
  }
  if (value >= 10_000) return `${(value / 1000).toFixed(1)}s`
  return `${Math.round(value)}ms`
}

export function summarizeModelHealth(rows: ModelHealthHourlyStat[]) {
  if (!Array.isArray(rows) || rows.length === 0) {
    return {
      avgRate: 0,
      totalSuccess: 0,
      totalSlices: 0,
      minRate: 0,
      maxRate: 0,
      totalRequests: 0,
      errorRequests: 0,
      successRequests: 0,
    }
  }

  let totalSuccess = 0
  let totalSlices = 0
  let minRate = 1
  let maxRate = 0
  let totalRequests = 0
  let errorRequests = 0
  let successRequests = 0
  let hasRateSample = false

  for (const row of rows) {
    totalSuccess += Number(row.success_slices) || 0
    totalSlices += Number(row.total_slices) || 0
    if ((Number(row.total_requests) || 0) > 0) {
      const rate = Number(row.success_rate) || 0
      if (rate < minRate) minRate = rate
      if (rate > maxRate) maxRate = rate
      hasRateSample = true
    }
    totalRequests += Number(row.total_requests) || 0
    errorRequests += Number(row.error_requests) || 0
    successRequests += Number(row.success_requests) || 0
  }

  const avgRate = totalRequests > 0 ? successRequests / totalRequests : 0
  return {
    avgRate,
    totalSuccess,
    totalSlices,
    minRate: hasRateSample ? minRate : 0,
    maxRate: hasRateSample ? maxRate : 0,
    totalRequests,
    errorRequests,
    successRequests,
  }
}
