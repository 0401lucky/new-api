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
import { z } from 'zod'

import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

export const donationSubmissionSchema = z.object({
  campaign_id: z.number().int().positive('Choose a donation campaign.'),
  keys_text: z
    .string()
    .refine((text) => text.trim().length > 0, 'Enter at least one API key.')
    .refine(
      (text) => text.split('\n').filter((line) => line.trim()).length <= 100,
      'Submit at most 100 keys at a time.'
    )
    .refine(
      (text) =>
        new TextEncoder().encode(JSON.stringify({ keys_text: text })).length <
        1048500,
      'This submission is too large. Use fewer keys.'
    ),
})

export type DonationSubmissionValues = z.infer<typeof donationSubmissionSchema>

export const donationConnectionSchema = z.object({
  base_url: z.url('Enter a valid service URL.'),
  token: z
    .string()
    .refine(
      (value) => value === '' || /^[!-~]{32,256}$/.test(value),
      'Enter a 32–256 character integration credential.'
    ),
})

export const donationCampaignSchema = (originalQuota?: number) =>
  z.object({
    name: z
      .string()
      .trim()
      .min(1, 'Enter a campaign name.')
      .max(120, 'Campaign names can contain at most 120 characters.'),
    description: z
      .string()
      .refine(
        (value) => new TextEncoder().encode(value).length <= 4000,
        'Please shorten the campaign description.'
      ),
    group_id: z.number().int().positive('Choose an available group.'),
    reward_amount: z.string().refine((value) => {
      if (
        originalQuota !== undefined &&
        originalQuota > 0 &&
        Number.isSafeInteger(originalQuota) &&
        value === String(quotaUnitsToDollars(originalQuota))
      ) {
        return true
      }
      const amount = Number(value)
      const quota = parseQuotaFromDollars(amount)
      return (
        value.trim() !== '' &&
        Number.isFinite(amount) &&
        amount > 0 &&
        Number.isSafeInteger(quota) &&
        quota > 0
      )
    }, 'Enter a positive reward within the supported quota range.'),
    enabled: z.boolean(),
  })

export type DonationCampaignValues = z.infer<
  ReturnType<typeof donationCampaignSchema>
>
