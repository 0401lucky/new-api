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
  return full.slice(11, 13) + ':00'
}

export function formatTokens(value: number) {
  const n = Number(value) || 0
  if (!Number.isFinite(n) || n === 0) return '0'

  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 0,
  }).format(Math.trunc(n))
}

export function percentileNearestRank(values: number[], p: number) {
  const arr = (values || [])
    .filter((v) => Number.isFinite(v))
    .slice()
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
