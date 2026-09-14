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
export interface DonationSession {
  readonly userId: number
  readonly sid: string
  readonly key: string
}

export interface DonationCampaign {
  id: number
  name: string
  description: string
  reward_quota: number
  enabled: boolean
  available: boolean
  unavailable_reason: string
}

export interface ManagedCampaign {
  id: number
  version: number
  name: string
  description: string
  instance_id: string
  group_id: number
  group_name: string
  target_revision: string
  reward_quota: number
  enabled: boolean
  created_at_ms: number
  updated_at_ms: number
}

export interface DonationConnection {
  base_url: string
  configured: boolean
  instance_id: string
  source_id: string
  version: number
  updated_at_ms: number
}

export interface DonationGroup {
  id: number
  name: string
  channel_id: string
  connection_type: string
  enabled: boolean
  can_probe: boolean
  target_revision: string
  unavailable_reason: string
}

export type DonationIntakeState =
  | 'unconfirmed'
  | 'queued'
  | 'validating'
  | 'committing'
  | 'accepted'
  | 'retry_pending'
  | 'invalid'
  | 'existing'
  | 'duplicate'
export type DonationRewardState = 'none' | 'pending' | 'paused' | 'rewarded'

export interface DonationItem {
  id: string
  batch_id: string
  line: number
  key_mask: string
  state: DonationIntakeState
  reason_code: string
  retryable: boolean
  credential_id: number | null
  accepted_at_ms: number | null
  reward_state: DonationRewardState
  reward_reason: string
  rewarded_quota: number
  rewarded_at_ms: number | null
  created_at_ms: number
  updated_at_ms: number
}

export interface DonationBatch {
  id: string
  request_key: string
  user_id: number
  username: string
  linux_do_id: string
  campaign_id: number
  campaign_version: number
  campaign_name: string
  instance_id: string
  group_id: number
  group_name: string
  reward_quota: number
  reception_state: 'local_only' | 'unconfirmed' | 'confirmed'
  last_error: string
  created_at_ms: number
  updated_at_ms: number
}

export interface DonationBatchDetail extends DonationBatch {
  items: DonationItem[]
  summary: {
    total: number
    accepted: number
    invalid: number
    duplicate: number
    processing: number
    rewarded: number
    rewarded_quota: number
  }
}

export interface DonationRecord {
  item: DonationItem
  batch: DonationBatch
  reward: {
    id: string
    item_id: string
    user_id: number
    quota: number
    credited_at_ms: number
  } | null
  events?: Array<{
    id: number
    item_id: string
    state: string
    reason_code: string
    created_at_ms: number
  }>
}

export interface DonationPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface DonationRecordFilters {
  p: number
  page_size: number
  user_id?: number
  campaign_id?: number
  group_id?: number
  state?: string
  reward_state?: string
  from_at_ms?: number
  to_at_ms?: number
  item_id?: string
  credential_id?: number
}

export type CampaignWrite = Pick<
  ManagedCampaign,
  'name' | 'description' | 'group_id' | 'reward_quota' | 'enabled'
>
export interface DonationSubmission {
  campaign_id: number
  keys_text: string
}
