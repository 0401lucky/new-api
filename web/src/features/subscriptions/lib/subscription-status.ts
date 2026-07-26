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
import type { UserSubscription } from '../types'

export type EffectiveSubscriptionStatus = 'active' | 'expired' | 'cancelled'

export function getEffectiveSubscriptionStatus(
  sub: Pick<UserSubscription, 'status' | 'end_time'>,
  nowSec = Date.now() / 1000
): EffectiveSubscriptionStatus {
  if (sub.status === 'cancelled') return 'cancelled'
  if (
    sub.status === 'expired' ||
    ((sub.end_time || 0) > 0 && sub.end_time < nowSec)
  ) {
    return 'expired'
  }
  if (sub.status === 'active') return 'active'
  return 'expired'
}

export function getSubscriptionUsagePercent(
  amountUsed: number,
  amountTotal: number
): number | null {
  if (!amountTotal || amountTotal <= 0) return null
  const pct = (Number(amountUsed || 0) / amountTotal) * 100
  if (!Number.isFinite(pct)) return 0
  return Math.max(0, Math.min(100, pct))
}
