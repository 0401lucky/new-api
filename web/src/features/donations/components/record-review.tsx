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
import { useMutation, useQuery } from '@tanstack/react-query'
import { isCancel } from 'axios'
import type { TFunction } from 'i18next'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import {
  donationApi,
  donationQueryKey,
  donationSessionIsCurrent,
  DonationRequestError,
} from '../api'
import { useDonationLifetime } from '../hooks/use-donation-session'
import {
  reviewKindLabel,
  reviewReasonLabel,
  reviewUnavailableLabel,
  validationModeLabel,
} from '../lib/labels'
import { DONATION_REVIEW_LIMIT_DEFAULTS } from '../lib/schema'
import type {
  DonationItem,
  DonationRecord,
  DonationReviewAction,
  DonationReviewKind,
  DonationReviewIntent,
  DonationSession,
  DonationTestMetadata,
  DonationTestResult,
} from '../types'
import { DonationRecordTest, DonationTestSummary } from './record-test'

type ReviewDialog = DonationReviewKind | null
type ReviewError = { code: string } | { message: string }

interface PendingReviewIntent {
  actionId: string
  actorId: number
  intent: DonationReviewIntent
}

function decisionLabel(
  item: DonationItem,
  action: DonationReviewAction | undefined,
  t: TFunction
): string {
  if (action) return reviewKindLabel(action.kind, t)
  switch (item.review_state) {
    case 'approved':
      return t('Approved')
    case 'rejected':
      return t('Rejected')
    case 'expired':
      return t('Temporary storage expired')
    default:
      break
  }
  if (item.state === 'pending_review') return t('Pending review')
  if (item.state === 'rejected') return t('Rejected')
  if (item.state === 'accepted' && item.effective_mode === 'manual_review') {
    return t('Approved')
  }
  return '—'
}

export function DonationRecordReview(props: {
  session: DonationSession
  record: DonationRecord
  canReview: boolean
  canTest: boolean
  onChanged: () => void
}) {
  const { t } = useTranslation()
  const item = props.record.item
  const [dialog, setDialog] = useState<ReviewDialog>(null)
  const [note, setNote] = useState('')
  const [actionError, setActionError] = useState<ReviewError | null>(null)
  const [lastTest, setLastTest] = useState<DonationTestMetadata | null>(null)
  const [testing, setTesting] = useState(false)
  const [completedAction, setCompletedAction] =
    useState<DonationReviewAction | null>(null)
  const [awaitingAction, setAwaitingAction] = useState(
    Boolean(props.record.pending_review_action)
  )
  const pendingIntent = useRef<PendingReviewIntent | null>(null)
  const settledActionId = useRef('')
  const getSignal = useDonationLifetime(props.session, () => {
    pendingIntent.current = null
  })
  const restoredAction = props.record.pending_review_action
  useEffect(() => {
    if (
      !restoredAction ||
      restoredAction.status !== 'pending' ||
      pendingIntent.current ||
      restoredAction.action_id === settledActionId.current
    ) {
      return
    }
    pendingIntent.current = {
      actionId: restoredAction.action_id,
      actorId: restoredAction.actor_id,
      intent: {
        kind: restoredAction.kind,
        expected_item_revision: restoredAction.expected_item_revision,
        ...(restoredAction.kind === 'reject'
          ? {}
          : { review_target_revision: restoredAction.review_target_revision }),
        ...(restoredAction.note ? { note: restoredAction.note } : {}),
      },
    }
    setAwaitingAction(true)
  }, [restoredAction])
  const context = useQuery({
    queryKey: donationQueryKey(props.session, 'review-context', item.id),
    queryFn: ({ signal }) =>
      donationApi.reviewContext(props.session, item.id, signal),
    enabled:
      item.state === 'pending_review' ||
      item.state === 'retry_pending' ||
      item.effective_mode === 'manual_review',
    refetchInterval: (query) =>
      query.state.data?.unavailable_reason === 'test_running' ? 2000 : false,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  // The decision, actor and time are projected from the applied action ledger,
  // never from a separately editable "reviewed" flag.
  const action = useQuery({
    queryKey: donationQueryKey(
      props.session,
      'review-action',
      item.id,
      item.review_action_id
    ),
    queryFn: ({ signal }) =>
      donationApi.reviewAction(
        props.session,
        item.id,
        item.review_action_id ?? '',
        signal
      ),
    enabled: Boolean(item.review_action_id),
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const applied =
    action.data?.status === 'applied'
      ? action.data
      : (completedAction ?? undefined)
  const limits = context.data?.review_limits ?? DONATION_REVIEW_LIMIT_DEFAULTS
  const noteTooLong =
    new TextEncoder().encode(note).length > limits.max_note_bytes
  const reviewNote =
    item.review_note ||
    (applied?.kind === 'approve' ? (applied.note ?? '') : '') ||
    '—'
  const openDialog = (next: ReviewDialog) => {
    if (pendingIntent.current) return
    setNote('')
    setActionError(null)
    setDialog(next)
  }
  const mutation = useMutation({
    mutationFn: async (kind: DonationReviewKind | 'reconcile') => {
      if (kind === 'reconcile') {
        const pending = pendingIntent.current
        if (!pending) throw new Error('The review context is not available.')
        try {
          return await donationApi.reviewAction(
            props.session,
            item.id,
            pending.actionId,
            getSignal()
          )
        } catch (error) {
          // Only the original actor may retry an intent that was never stored.
          // A confirmed/pending action is read back without another POST.
          if (
            !(error instanceof DonationRequestError) ||
            error.status !== 404 ||
            pending.actorId !== props.session.userId ||
            !props.canReview
          ) {
            throw error
          }
          return donationApi.createReviewAction(
            props.session,
            item.id,
            pending.actionId,
            pending.intent,
            getSignal()
          )
        }
      }
      const current = context.data
      if (!current) throw new Error('The review context is not available.')
      if (pendingIntent.current) {
        throw new Error('A review action for this record is already pending.')
      }
      if (noteTooLong) throw new Error('Please shorten this note.')
      const pending: PendingReviewIntent = {
        actionId: crypto.randomUUID(),
        actorId: props.session.userId,
        intent: {
          kind,
          expected_item_revision: current.item_revision,
          ...(kind === 'reject'
            ? {}
            : { review_target_revision: current.review_target_revision }),
          ...(note.trim() ? { note: note.trim() } : {}),
        },
      }
      pendingIntent.current = pending
      setAwaitingAction(true)
      return donationApi.createReviewAction(
        props.session,
        item.id,
        pending.actionId,
        pending.intent,
        getSignal()
      )
    },
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
    onSuccess: (result) => {
      if (!donationSessionIsCurrent(props.session) || getSignal().aborted) {
        return
      }
      setDialog(null)
      setNote('')
      if (result.status !== 'pending') {
        pendingIntent.current = null
        settledActionId.current = result.action_id
        setAwaitingAction(false)
        if (result.status === 'applied') setCompletedAction(result)
      }
      setActionError(
        result.status === 'rejected' ? { code: result.reason_code } : null
      )
      void context.refetch()
      if (item.review_action_id) void action.refetch()
      props.onChanged()
    },
    onError: (error) => {
      if (
        !donationSessionIsCurrent(props.session) ||
        getSignal().aborted ||
        isCancel(error)
      ) {
        return
      }
      // These failures occur before a write is authorized or validated. All
      // other failures retain the immutable intent until its outcome is known.
      if (
        error instanceof DonationRequestError &&
        [400, 401, 403, 422].includes(error.status)
      ) {
        pendingIntent.current = null
        setAwaitingAction(false)
      }
      setDialog(null)
      setNote('')
      setActionError(
        error instanceof DonationRequestError && error.status === 409
          ? { message: 'This record cannot be acted on right now.' }
          : { message: error.message }
      )
      props.onChanged()
    },
  })
  const refetchContext = context.refetch
  const onChanged = props.onChanged
  const recentActions = props.record.recent_review_actions
  useEffect(() => {
    const pending = pendingIntent.current
    if (!pending) return
    let confirmed = action.data
    if (
      confirmed?.action_id !== pending.actionId ||
      confirmed.actor_id !== pending.actorId
    ) {
      confirmed = recentActions?.find(
        (entry) =>
          entry.action_id === pending.actionId &&
          entry.actor_id === pending.actorId
      )
    }
    if (!confirmed || confirmed.status === 'pending') return
    // Clearing a nullable pending projection alone is not proof. Only the
    // matching durable action, including its original actor, settles this UI.
    pendingIntent.current = null
    settledActionId.current = confirmed.action_id
    setAwaitingAction(false)
    if (confirmed.status === 'applied') setCompletedAction(confirmed)
    setActionError(
      confirmed.status === 'rejected' ? { code: confirmed.reason_code } : null
    )
    void refetchContext()
    onChanged()
  }, [action.data, recentActions, refetchContext, onChanged])
  const onTestFinished = useCallback(() => {
    void refetchContext()
    onChanged()
  }, [refetchContext, onChanged])
  const onTestResult = useCallback((result: DonationTestResult) => {
    const metadata = { ...result }
    delete metadata.text
    setLastTest(metadata)
  }, [])
  const manual =
    context.data?.effective_mode === 'manual_review' &&
    context.data.state === 'pending_review'
  const canDecide =
    props.canReview &&
    !mutation.isPending &&
    !awaitingAction &&
    !testing &&
    !context.isFetching &&
    !(
      completedAction?.status === 'applied' &&
      completedAction.kind !== 'enter_review'
    )
  const reviewable = Boolean(
    canDecide &&
    manual &&
    context.data?.can_review &&
    context.data.review_action === 'approve'
  )
  const canEnterReview = Boolean(
    canDecide &&
    context.data?.review_action === 'enter_review' &&
    context.data?.can_review &&
    context.data.state === 'retry_pending'
  )
  const canReject = Boolean(canDecide && manual && context.data?.can_reject)
  const latestTest =
    !lastTest ||
    (props.record.latest_test?.started_at_ms ?? 0) > lastTest.started_at_ms
      ? props.record.latest_test
      : lastTest
  const targetChanged =
    context.data?.unavailable_reason === 'target_changed' ||
    item.reason_code === 'target_changed'
  let targetLabel = '—'
  if (targetChanged) {
    targetLabel = t(
      'The receiving target changed after this key was submitted.'
    )
  } else if (
    context.data?.unavailable_reason &&
    ['target_unavailable', 'group_deleted'].includes(
      context.data.unavailable_reason
    )
  ) {
    targetLabel = reviewUnavailableLabel(context.data.unavailable_reason, t)
  } else if (context.data?.review_target_revision) {
    targetLabel = t('The receiving target is unchanged.')
  }
  return (
    <section
      aria-label={t('Manual review')}
      className='flex flex-col gap-3 rounded-lg border p-3'
    >
      <h3 className='font-medium'>{t('Manual review')}</h3>
      <dl className='grid grid-cols-1 gap-3 text-sm sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground'>{t('Validation mode')}</dt>
          <dd>
            {validationModeLabel(
              item.effective_mode ??
                context.data?.effective_mode ??
                props.record.batch.validation_mode,
              t
            )}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Review decision')}</dt>
          <dd>{decisionLabel(item, applied, t)}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Reviewed by')}</dt>
          <dd>
            {applied?.actor_id
              ? t('Administrator #{{id}}', { id: applied.actor_id })
              : '—'}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Reviewed at')}</dt>
          <dd>
            {formatTimestampToDate(
              applied?.applied_at_ms ?? item.reviewed_at_ms ?? undefined,
              'milliseconds'
            )}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Review note')}</dt>
          <dd className='break-words'>{reviewNote}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('Temporary storage expires')}
          </dt>
          <dd>
            {formatTimestampToDate(
              item.staging_expires_at_ms ?? context.data?.expires_at_ms,
              'milliseconds'
            )}
          </dd>
        </div>
        <div className='sm:col-span-2'>
          <dt className='text-muted-foreground'>{t('Receiving target')}</dt>
          <dd>{targetLabel}</dd>
        </div>
      </dl>
      <div className='flex flex-col gap-2'>
        <h4 className='text-sm font-medium'>{t('Latest test')}</h4>
        {latestTest ? (
          <>
            <DonationTestSummary test={latestTest} />
            {(targetChanged ||
              (context.data?.review_target_revision &&
                latestTest.target_revision !==
                  context.data.review_target_revision)) && (
              <p className='text-muted-foreground text-sm'>
                {t(
                  'This test used an earlier receiving target and is no longer current.'
                )}
              </p>
            )}
          </>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('No test has been run for this record.')}
          </p>
        )}
      </div>
      {action.isError && (
        <Alert variant='destructive'>
          <AlertDescription>{t(action.error.message)}</AlertDescription>
        </Alert>
      )}
      {context.isError && (
        <Alert variant='destructive'>
          <AlertDescription>{t(context.error.message)}</AlertDescription>
        </Alert>
      )}
      {context.data && context.data.unavailable_reason && (
        <Alert>
          <AlertDescription>
            {reviewUnavailableLabel(context.data.unavailable_reason, t)}
          </AlertDescription>
        </Alert>
      )}
      {actionError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {'code' in actionError
              ? reviewReasonLabel(actionError.code, t)
              : t(actionError.message)}
          </AlertDescription>
        </Alert>
      )}
      {awaitingAction && (
        <Alert>
          <AlertDescription className='flex flex-wrap items-center justify-between gap-2'>
            <span>
              {t(
                'The review result is not confirmed. Check the original action before making another decision.'
              )}
            </span>
            <Button
              variant='outline'
              disabled={mutation.isPending}
              onClick={() => mutation.mutate('reconcile')}
            >
              {t('Check review result')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {props.canReview && context.data && (
        <div className='flex flex-wrap items-center gap-2'>
          {canEnterReview && (
            <Button
              variant='outline'
              onClick={() => openDialog('enter_review')}
            >
              {t('Move to manual review')}
            </Button>
          )}
          {reviewable && (
            <Button onClick={() => openDialog('approve')}>
              {t('Approve')}
            </Button>
          )}
          {canReject && (
            <Button variant='destructive' onClick={() => openDialog('reject')}>
              {t('Reject')}
            </Button>
          )}
        </div>
      )}
      {props.canTest && manual && context.data && (
        <DonationRecordTest
          session={props.session}
          itemId={item.id}
          context={context.data}
          disabled={awaitingAction || mutation.isPending}
          onResult={onTestResult}
          onRunningChange={setTesting}
          onFinished={onTestFinished}
        />
      )}
      {props.canTest && context.data && !context.data.can_test && (
        <p className='text-muted-foreground text-sm'>
          {t('This record cannot be tested right now.')}
        </p>
      )}
      <ConfirmDialog
        open={dialog === 'enter_review'}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDialog(null)
        }}
        title={t('Move to manual review')}
        desc={t(
          'This failed automatic validation. Sending it to manual review keeps the original submission, deadline and reward.'
        )}
        confirmText={t('Move to manual review')}
        disabled={!canEnterReview}
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate('enter_review')}
      />
      <ConfirmDialog
        open={dialog === 'approve'}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDialog(null)
        }}
        title={t('Approve this donation')}
        desc={
          <div className='flex flex-col gap-2'>
            <span>
              {t(
                'The permanent reward is credited to the donor once the key is received.'
              )}
            </span>
            <dl className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1'>
              <dt className='text-muted-foreground'>{t('User')}</dt>
              <dd>
                {props.record.batch.username} · {props.record.batch.user_id}
              </dd>
              <dt className='text-muted-foreground'>{t('Key')}</dt>
              <dd className='font-mono'>{item.key_mask}</dd>
              <dt className='text-muted-foreground'>
                {t('Permanent reward per key')}
              </dt>
              <dd>{formatQuota(props.record.batch.reward_quota)}</dd>
            </dl>
          </div>
        }
        confirmText={t('Approve')}
        disabled={noteTooLong || !reviewable}
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate('approve')}
      >
        <div className='flex flex-col gap-2'>
          <Label htmlFor='donation-approve-note'>{t('Note (optional)')}</Label>
          <Textarea
            id='donation-approve-note'
            value={note}
            onChange={(event) => setNote(event.target.value)}
            rows={2}
            disabled={mutation.isPending}
          />
          {noteTooLong && (
            <p role='alert' className='text-destructive text-sm'>
              {t('Please shorten this note.')}
            </p>
          )}
        </div>
      </ConfirmDialog>
      <ConfirmDialog
        open={dialog === 'reject'}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setDialog(null)
        }}
        title={t('Reject this donation')}
        desc={t(
          'The donor will see this reason. The temporarily stored key is cleared and no reward is credited.'
        )}
        confirmText={t('Reject')}
        destructive
        disabled={note.trim() === '' || noteTooLong || !canReject}
        isLoading={mutation.isPending}
        handleConfirm={() => mutation.mutate('reject')}
      >
        <div className='flex flex-col gap-2'>
          <Label htmlFor='donation-reject-note'>
            {t('Reason the donor can read')}
          </Label>
          <Textarea
            id='donation-reject-note'
            value={note}
            onChange={(event) => setNote(event.target.value)}
            rows={3}
            disabled={mutation.isPending}
          />
          {noteTooLong ? (
            <p role='alert' className='text-destructive text-sm'>
              {t('Please shorten this note.')}
            </p>
          ) : null}
        </div>
      </ConfirmDialog>
    </section>
  )
}
