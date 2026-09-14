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
import { useCallback, useEffect, useMemo, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import { donationSessionIsCurrent } from '../api'
import type { DonationSession } from '../types'

export function useDonationSession(): DonationSession {
  const userId = useAuthStore((state) => state.auth.user?.id ?? 0)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? '')
  return useMemo(
    () => ({ userId, sid, key: `${String(userId)}:${sid}` }),
    [userId, sid]
  )
}

/** Cancel on unmount as well as on login changes; callers keep secrets in refs,
 * not mutation variables, and this cleanup also erases those refs/forms. */
export function useDonationLifetime(
  session: DonationSession,
  clear: () => void
): () => AbortSignal {
  const clearRef = useRef(clear)
  clearRef.current = clear
  const controllerRef = useRef(new AbortController())
  useEffect(() => {
    if (controllerRef.current.signal.aborted) {
      controllerRef.current = new AbortController()
    }
    const controller = controllerRef.current
    const unsubscribe = useAuthStore.subscribe(() => {
      if (!donationSessionIsCurrent(session)) {
        controller.abort()
        clearRef.current()
      }
    })
    return () => {
      controller.abort()
      clearRef.current()
      unsubscribe()
    }
  }, [session])
  return useCallback(() => controllerRef.current.signal, [])
}
