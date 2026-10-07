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
import type { TFunction } from 'i18next'
import { z } from 'zod'

export type MultiplierMode = 'fixed' | 'balance' | 'concurrency'
export interface ConcurrencyTier {
  minimum: number
  multiplier: number
}
export interface GroupMultiplierPolicy {
  mode: MultiplierMode
  tiers: ConcurrencyTier[]
}
export interface GroupMultiplierStatus {
  group: string
  description: string
  policy: GroupMultiplierPolicy
  version: string
  base_ratio: number
  concurrency: number | null
  factor: number | null
  effective_ratio: number | null
  next_request_ratio: number | null
  next_tier: ConcurrencyTier | null
}

export function multiplierModeLabel(
  mode: MultiplierMode,
  t: TFunction
): string {
  if (mode === 'balance') return t('Balance multiplier')
  if (mode === 'concurrency') return t('Concurrency multiplier')
  return t('Fixed multiplier')
}

// Multipliers need more precision than ordinary two-decimal number displays.
export function formatMultiplier(
  value: number | null,
  locale?: string
): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `×${new Intl.NumberFormat(locale, { maximumSignificantDigits: 12 }).format(value)}`
}

export function groupPolicySchema(t: TFunction) {
  return z
    .object({
      mode: z.enum(['fixed', 'balance', 'concurrency']),
      tiers: z.array(
        z.object({
          minimum: z.number({ error: t('Enter a valid number') }),
          multiplier: z.number({ error: t('Enter a valid number') }),
        })
      ),
    })
    .superRefine((policy, ctx) => {
      if (
        policy.tiers.length > 32 ||
        (policy.mode === 'concurrency' && policy.tiers.length === 0)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['tiers'],
          message: t('Configure 1 to 32 concurrency tiers.'),
        })
      }
      policy.tiers.forEach((tier, index) => {
        const previous = policy.tiers[index - 1]
        if (
          !Number.isInteger(tier.minimum) ||
          tier.minimum < 0 ||
          tier.minimum > 1000000 ||
          (index === 0 && tier.minimum !== 0) ||
          (previous && tier.minimum <= previous.minimum)
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['tiers', index, 'minimum'],
            message: t(
              'Thresholds must start at 0 and increase, up to 1000000.'
            ),
          })
        }
        if (
          !Number.isFinite(tier.multiplier) ||
          tier.multiplier <= 0 ||
          tier.multiplier > 1000 ||
          (previous && tier.multiplier < previous.multiplier)
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['tiers', index, 'multiplier'],
            message: t(
              'Multipliers must be positive, non-decreasing, and at most 1000.'
            ),
          })
        }
      })
    })
}
