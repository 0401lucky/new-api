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
import { useEffect } from 'react'
import FingerprintJS from '@fingerprintjs/fingerprintjs'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

const REPORT_INTERVAL_MS = 60 * 60 * 1000
const STORAGE_KEY_PREFIX = 'new-api:fingerprint:last-report:'

let fingerprintAgentPromise: ReturnType<typeof FingerprintJS.load> | null =
  null

function loadFingerprintAgent() {
  if (!fingerprintAgentPromise) {
    fingerprintAgentPromise = FingerprintJS.load()
  }
  return fingerprintAgentPromise
}

function shouldReport(userId: number) {
  if (typeof window === 'undefined') return false
  const key = `${STORAGE_KEY_PREFIX}${userId}`
  const lastReportAt = Number(window.localStorage.getItem(key) || '0')
  return Date.now() - lastReportAt > REPORT_INTERVAL_MS
}

function markReported(userId: number) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(`${STORAGE_KEY_PREFIX}${userId}`, String(Date.now()))
}

export function useFingerprintReport() {
  const userId = useAuthStore((state) => state.auth.user?.id)

  useEffect(() => {
    if (!userId || !shouldReport(userId)) {
      return
    }

    let cancelled = false
    const currentUserId = userId

    async function reportFingerprint() {
      const agent = await loadFingerprintAgent()
      const result = await agent.get()
      if (cancelled || !result.visitorId) {
        return
      }

      await api.post(
        '/api/fingerprint/record',
        { visitor_id: result.visitorId },
        {
          skipBusinessError: true,
          skipErrorHandler: true,
        }
      )

      if (!cancelled) {
        markReported(currentUserId)
      }
    }

    void reportFingerprint().catch(() => {
      // 指纹采集只作为风控线索，失败时不打扰用户正常使用。
    })

    return () => {
      cancelled = true
    }
  }, [userId])
}
