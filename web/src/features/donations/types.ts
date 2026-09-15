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
  validation_mode: DonationValidationMode
}

export type DonationValidationMode = 'auto' | 'manual_review'

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
  validation_mode: DonationValidationMode
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
  /** Added by the `manual_review_v1` integration feature. Absent on older receivers. */
  can_manual_review?: boolean
  manual_target_revision?: string
  manual_unavailable_reason?: string
}

export type DonationIntakeState =
  | 'unconfirmed'
  | 'queued'
  | 'validating'
  | 'committing'
  | 'accepted'
  | 'retry_pending'
  | 'pending_review'
  | 'rejected'
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
  /** Manual-review facts, independent from the automatic probe state. */
  effective_mode?: DonationValidationMode
  item_revision?: number
  review_target_revision?: string
  /** pending_review | approved | rejected | expired, empty outside manual review. */
  review_state?: string
  review_action_id?: string
  /** Donor-visible note of the final applied reject action. */
  review_note?: string
  reviewed_at_ms?: number | null
  staging_expires_at_ms?: number | null
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
  validation_mode?: DonationValidationMode
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
    pending_review?: number
    rejected?: number
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
  pending_review_action?: DonationReviewAction | null
  latest_test?: DonationTestMetadata | null
  recent_review_actions?: DonationReviewAction[]
  recent_tests?: DonationTestMetadata[]
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
> & {
  validation_mode: DonationValidationMode
}

export type DonationReviewKind = 'enter_review' | 'approve' | 'reject'

export interface DonationReviewLimits {
  max_prompt_bytes: number
  max_request_bytes: number
  default_output_tokens: number
  max_output_tokens: number
  max_response_bytes: number
  max_event_bytes: number
  total_timeout_seconds: number
  first_byte_timeout_seconds: number
  idle_timeout_seconds: number
  max_note_bytes: number
}

export interface DonationReviewContext {
  batch_id: string
  item_id: string
  group_id: number
  state: DonationIntakeState
  effective_mode: DonationValidationMode
  item_revision: number
  review_target_revision: string
  expires_at_ms: number
  can_review: boolean
  can_reject: boolean
  can_test: boolean
  review_action: '' | 'enter_review' | 'approve'
  unavailable_reason: string
  test_models: string[]
  review_limits: DonationReviewLimits
}

export interface DonationReviewIntent {
  kind: DonationReviewKind
  expected_item_revision: number
  review_target_revision?: string
  note?: string
}

/** new-api's ledger projection of one review intent. `status` is the command
 * outcome: `rejected` means the command was not applied, not that the donation
 * was rejected. */
export interface DonationReviewAction {
  action_id: string
  batch_id: string
  item_id: string
  actor_id: number
  kind: DonationReviewKind
  expected_item_revision: number
  review_target_revision: string
  status: 'pending' | 'applied' | 'rejected'
  reason_code: string
  effect_revision: number
  note?: string
  applied_at_ms: number | null
  created_at_ms: number
}

export type DonationTestState =
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'interrupted'

export interface DonationTestUsage {
  input_tokens?: number
  output_tokens?: number
}

export interface DonationTestMetadata {
  test_id: string
  actor_id?: number
  batch_id: string
  item_id: string
  model: string
  stream: boolean
  state: DonationTestState
  reason_code: string
  status_code: number
  start_revision: number
  target_revision: string
  started_at_ms: number
  finished_at_ms: number | null
  output_bytes: number
  input_tokens?: number
  output_tokens?: number
  usage?: DonationTestUsage
}

export interface DonationTestResult extends DonationTestMetadata {
  text?: string
}

export interface DonationTestIntent {
  expected_item_revision: number
  review_target_revision: string
  model: string
  prompt: string
  system_prompt?: string
  max_output_tokens?: number
  stream: boolean
}
export interface DonationSubmission {
  campaign_id: number
  keys_text: string
}
