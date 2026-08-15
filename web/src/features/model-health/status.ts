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
import type { ModelHealthGlobalStatus, ModelHealthStatus } from './types'

export type StatusMeta = {
  labelKey: string
  dotClass: string
  badgeClass: string
  barClass: string
}

export const STATUS_META: Record<ModelHealthStatus, StatusMeta> = {
  operational: {
    labelKey: 'Operational',
    dotClass: 'bg-emerald-500',
    badgeClass:
      'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
    barClass: 'bg-emerald-500',
  },
  degraded: {
    labelKey: 'Degraded',
    dotClass: 'bg-amber-500',
    badgeClass:
      'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
    barClass: 'bg-amber-500',
  },
  outage: {
    labelKey: 'Outage',
    dotClass: 'bg-red-500',
    badgeClass:
      'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400',
    barClass: 'bg-red-500',
  },
  no_data: {
    labelKey: 'No data',
    dotClass: 'bg-slate-300 dark:bg-slate-600',
    badgeClass:
      'border-slate-300/60 bg-slate-100 text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400',
    barClass: 'bg-slate-200 dark:bg-slate-700/80',
  },
}

export const GLOBAL_STATUS_META: Record<
  ModelHealthGlobalStatus,
  { labelKey: string; dotClass: string; badgeClass: string }
> = {
  operational: {
    labelKey: 'All systems operational',
    dotClass: 'bg-emerald-500',
    badgeClass:
      'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  },
  degraded: {
    labelKey: 'Partial degradation',
    dotClass: 'bg-amber-500',
    badgeClass:
      'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  },
  outage: {
    labelKey: 'Service disruption',
    dotClass: 'bg-red-500',
    badgeClass:
      'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400',
  },
}

// 时间线小时格与后端状态判定使用同一套阈值（95% / 80%）。
export function timelineStatusFromRate(
  totalRequests: number,
  rate: number
): ModelHealthStatus {
  if (!totalRequests || totalRequests <= 0) return 'no_data'
  const v = Number(rate) || 0
  if (v >= 0.95) return 'operational'
  if (v >= 0.8) return 'degraded'
  return 'outage'
}

// 可用性百分比配色阈值沿 check-cx：>=99% 绿、>=95% 黄、<95% 红。
export function availabilityColorClass(availability: number | null): string {
  if (availability === null || !Number.isFinite(availability)) {
    return 'text-muted-foreground'
  }
  if (availability >= 0.99) return 'text-emerald-600 dark:text-emerald-400'
  if (availability >= 0.95) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}
