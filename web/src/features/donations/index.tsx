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
import { useCallback, useState } from 'react'

import { LoadingState } from '@/components/loading-state'

import { DonationBatchHistory } from './components/batch-history'
import { DonationBatchResults } from './components/batch-results'
import { DonationLayout } from './components/donation-layout'
import { DonationSubmissionForm } from './components/submission-form'
import { useDonationSession } from './hooks/use-donation-session'
import type { DonationBatchDetail, DonationSession } from './types'

export function Donations() {
  const session = useDonationSession()
  return (
    <DonationLayout page='donate'>
      {session.userId && session.sid ? (
        <DonationWorkspace key={session.key} session={session} />
      ) : (
        <LoadingState />
      )}
    </DonationLayout>
  )
}

function DonationWorkspace(props: { session: DonationSession }) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [resume, setResume] = useState<DonationBatchDetail | null>(null)
  const [observedBatch, setObservedBatch] =
    useState<DonationBatchDetail | null>(null)
  const onResult = useCallback((batch: DonationBatchDetail) => {
    setSelectedId(batch.id)
    setObservedBatch(batch)
    setResume(batch.reception_state === 'unconfirmed' ? batch : null)
  }, [])
  const onObserved = useCallback((batch: DonationBatchDetail) => {
    setObservedBatch(batch)
    if (batch.reception_state !== 'unconfirmed') {
      setResume((previous) => (previous?.id === batch.id ? null : previous))
    }
  }, [])
  return (
    <div className='mx-auto flex w-full max-w-6xl flex-col gap-6 pb-4'>
      <DonationSubmissionForm
        session={props.session}
        resume={resume}
        observedBatch={observedBatch}
        onResult={onResult}
        onNew={() => setResume(null)}
      />
      {selectedId && (
        <DonationBatchResults
          key={selectedId}
          session={props.session}
          batchId={selectedId}
          onResume={setResume}
          onObserved={onObserved}
        />
      )}
      <DonationBatchHistory session={props.session} onOpen={setSelectedId} />
    </div>
  )
}
