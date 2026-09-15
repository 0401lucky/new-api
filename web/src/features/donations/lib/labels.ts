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

import type { StatusVariant } from '@/components/status-badge'

import type { DonationReviewKind, DonationValidationMode } from '../types'

export function intakeLabel(state: string, t: TFunction): string {
  switch (state) {
    case 'unconfirmed':
      return t('Receipt unconfirmed')
    case 'queued':
      return t('Queued')
    case 'validating':
      return t('Validating')
    case 'committing':
      return t('Receiving key')
    case 'accepted':
      return t('Accepted')
    case 'retry_pending':
      return t('Retry available')
    case 'pending_review':
      return t('Pending review')
    case 'rejected':
      return t('Review rejected')
    case 'invalid':
      return t('Invalid')
    case 'existing':
      return t('Already exists')
    case 'duplicate':
      return t('Duplicate key')
    case 'reward_paused':
      return t('Reward paused')
    case 'rewarded':
      return t('Reward credited')
    default:
      return t('Unknown')
  }
}

export function intakeVariant(state: string): StatusVariant {
  if (state === 'accepted' || state === 'rewarded') return 'success'
  if (state === 'invalid' || state === 'rejected') return 'danger'
  if (
    state === 'unconfirmed' ||
    state === 'retry_pending' ||
    state === 'pending_review' ||
    state === 'reward_paused'
  ) {
    return 'warning'
  }
  if (state === 'duplicate' || state === 'existing') return 'neutral'
  return 'info'
}

export function testStateLabel(state: string, t: TFunction): string {
  switch (state) {
    case 'succeeded':
      return t('Test succeeded')
    case 'failed':
      return t('Test failed')
    case 'cancelled':
      return t('Test cancelled')
    case 'interrupted':
      return t('Test interrupted')
    default:
      return t('Test running')
  }
}

export function testReasonLabel(reason: string, t: TFunction): string {
  switch (reason) {
    case 'cancelled':
      return t('Test cancelled')
    case 'interrupted':
    case 'lease_expired':
      return t('Test interrupted')
    case 'timeout':
    case 'first_byte_timeout':
    case 'idle_timeout':
      return t('The model test timed out before it reported a result.')
    case 'response_too_large':
    case 'output_limit':
      return t('The test response exceeded the supported size limit.')
    case 'model_unavailable':
    case 'target_changed':
    case 'target_unavailable':
    case 'staging_expired':
    case 'test_running':
      return reviewUnavailableLabel(reason, t)
    default:
      return t(
        'The model test did not complete successfully. This alone does not prove the key is unusable.'
      )
  }
}

export function validationModeLabel(
  mode: DonationValidationMode | undefined,
  t: TFunction
): string {
  return mode === 'manual_review'
    ? t('Manual review')
    : t('Automatic validation')
}

export function reviewKindLabel(
  kind: DonationReviewKind | '' | undefined,
  t: TFunction
): string {
  switch (kind) {
    case 'approve':
      return t('Approved')
    case 'reject':
      return t('Rejected')
    case 'enter_review':
      return t('Moved to manual review')
    default:
      return t('Not reviewed yet')
  }
}

/** Why the review context refuses an action right now. */
export function reviewUnavailableLabel(reason: string, t: TFunction): string {
  switch (reason) {
    case 'staging_expired':
      return t(
        'The temporarily stored key expired. This record can no longer be approved.'
      )
    case 'item_state':
      return t('This record is not waiting for manual review.')
    case 'target_changed':
      return t(
        'The receiving target changed after this key was submitted. This record cannot be tested or approved.'
      )
    case 'target_unavailable':
      return t('The receiving target is currently unavailable.')
    case 'group_deleted':
      return t('This group no longer exists.')
    case 'test_running':
      return t('A model test is already running for this record.')
    case 'resource_busy':
      return t('This key is busy in another operation. Try again shortly.')
    case 'model_unavailable':
      return t(
        'The group has no compatible text model, so this record cannot be tested. It can still be reviewed manually.'
      )
    default:
      return t('This record cannot be acted on right now.')
  }
}

/** Business codes returned by a review command that did not apply. */
export function reviewReasonLabel(reason: string, t: TFunction): string {
  switch (reason) {
    case '':
      return ''
    case 'revision_mismatch':
      return t('This record changed after the page was loaded. Reload it.')
    case 'mode_conflict':
      return t('This record is no longer handled by manual review.')
    case 'target_changed':
      return t(
        'The receiving target changed after submission. This record can only be rejected.'
      )
    case 'target_unavailable':
      return t('The receiving target is currently unavailable.')
    case 'staging_expired':
      return t('The unaccepted key expired from temporary storage.')
    case 'item_state':
      return t('This record is not waiting for manual review.')
    case 'already_reviewed':
      return t('Another administrator already reviewed this record.')
    case 'not_reviewable':
      return t('This record cannot be reviewed manually.')
    case 'test_running':
      return t('A model test is still running for this record.')
    case 'resource_busy':
      return t('This key is busy in another operation. Try again shortly.')
    case 'duplicate_pending':
      return t('A review action for this record is already pending.')
    default:
      return t('The review action could not be applied.')
  }
}

export function rewardLabel(state: string, t: TFunction): string {
  switch (state) {
    case 'rewarded':
      return t('Reward credited')
    case 'pending':
      return t('Reward pending')
    case 'paused':
      return t('Reward paused')
    default:
      return t('No reward')
  }
}

export function reasonLabel(reason: string, t: TFunction): string {
  switch (reason) {
    case '':
      return ''
    case 'invalid_format':
      return t('This line is not a valid API key.')
    case 'review_rejected':
      return t('Review rejected')
    case 'duplicate_item':
      return t('This key appears more than once in this submission.')
    case 'already_exists':
    case 'already_donated':
      return t('This key already exists and does not receive a new reward.')
    case 'resource_processing':
    case 'resource_busy':
      return t('This key is already being processed.')
    case 'invalid_credential':
      return t('The upstream service rejected this key.')
    case 'rate_limited':
      return t('The upstream service is rate limiting validation.')
    case 'timeout':
      return t('Validation timed out. No reward has been issued yet.')
    case 'retry_exhausted':
      return t(
        'Automatic validation retries are exhausted. You can retry this submission.'
      )
    case 'staging_expired':
      return t(
        'The unaccepted key expired from temporary storage. Submit it again if needed.'
      )
    case 'group_disabled':
      return t('This group is disabled.')
    case 'group_deleted':
      return t('This group no longer exists.')
    case 'target_changed':
    case 'target_or_request_conflict':
      return t('The selected group configuration changed.')
    case 'unsupported_input':
    case 'credential_override':
    case 'probe_unavailable':
    case 'probe_incompatible':
    case 'target_unavailable':
    case 'model_unavailable':
      return t('This group cannot currently validate API keys.')
    case 'account_disabled':
      return t('The reward is paused while the account is disabled.')
    case 'wallet_limit':
      return t('The reward is waiting for available wallet capacity.')
    case 'reward_pending':
      return t('The permanent reward will be credited automatically.')
    case 'runtime_pending':
      return t('The key is being made available for use.')
    case 'receipt_not_found':
      return t(
        'Receipt is not confirmed. Keep the original keys and retry this submission.'
      )
    case 'instance_mismatch':
    case 'identity_unavailable':
      return t('The integration identity needs administrator attention.')
    case 'campaign_closed':
      return t('This campaign is closed.')
    default:
      return t('Processing is temporarily unavailable. Refresh or retry later.')
  }
}
