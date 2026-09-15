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

import type { DonationReviewLimits } from '../types'

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
    manual_review: z.boolean(),
  })

export type DonationCampaignValues = z.infer<
  ReturnType<typeof donationCampaignSchema>
>

/** First-version review limits, used until the receiver reports its own. */
export const DONATION_REVIEW_LIMIT_DEFAULTS: DonationReviewLimits = {
  max_prompt_bytes: 16384,
  max_request_bytes: 65536,
  default_output_tokens: 1024,
  max_output_tokens: 4096,
  max_response_bytes: 131072,
  max_event_bytes: 65536,
  total_timeout_seconds: 120,
  first_byte_timeout_seconds: 30,
  idle_timeout_seconds: 20,
  max_note_bytes: 2048,
}

function utf8Bytes(value: string): number {
  return new TextEncoder().encode(value).length
}

export function donationReviewNoteSchema(maxBytes: number) {
  return z
    .string()
    .refine(
      (value) => utf8Bytes(value) <= maxBytes,
      'Please shorten this note.'
    )
}

export function donationTestSchema(limits: DonationReviewLimits) {
  return z
    .object({
      model: z.string().min(1, 'Choose a text model.'),
      prompt: z
        .string()
        .refine((value) => value.trim() !== '', 'Enter a prompt for the test.'),
      system_prompt: z.string(),
      max_output_tokens: z
        .number()
        .int()
        .min(1, 'Enter at least one output token.')
        .max(
          limits.max_output_tokens,
          'This exceeds the supported output token limit.'
        ),
      stream: z.boolean(),
    })
    .refine(
      (value) =>
        utf8Bytes(value.prompt) + utf8Bytes(value.system_prompt) <=
        limits.max_prompt_bytes,
      {
        path: ['prompt'],
        message: 'Please shorten the prompt or the system prompt.',
      }
    )
}

export type DonationTestValues = z.infer<ReturnType<typeof donationTestSchema>>

const testUsageSchema = z.object({
  input_tokens: z.number().int().nonnegative().optional(),
  output_tokens: z.number().int().nonnegative().optional(),
})

export const donationTestMetaSchema = z.object({
  test_id: z.uuid(),
  batch_id: z.uuid(),
  item_id: z.uuid(),
  model: z.string().min(1),
  start_revision: z.number().int().nonnegative(),
  target_revision: z.string().regex(/^[a-f0-9]{64}$/),
  started_at_ms: z.number().int().nonnegative(),
})

export const donationTestDoneSchema = z.object({
  state: z.enum(['succeeded', 'failed', 'cancelled', 'interrupted']),
  reason_code: z.string().max(256),
  status_code: z.number().int().min(0).max(599),
  finished_at_ms: z.number().int().nonnegative(),
  output_bytes: z.number().int().nonnegative(),
  usage: testUsageSchema.optional(),
})

export const donationTestResultSchema = donationTestMetaSchema.extend({
  stream: z.boolean(),
  state: z.enum(['running', 'succeeded', 'failed', 'cancelled', 'interrupted']),
  reason_code: z.string().max(256),
  status_code: z.number().int().min(0).max(599),
  finished_at_ms: z.number().int().nonnegative().nullable(),
  output_bytes: z.number().int().nonnegative(),
  usage: testUsageSchema.optional(),
  input_tokens: z.number().int().nonnegative().optional(),
  output_tokens: z.number().int().nonnegative().optional(),
  text: z.string().optional(),
})
