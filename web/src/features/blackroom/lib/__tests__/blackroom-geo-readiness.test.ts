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
import { assert, describe, test } from 'vitest'

import type { BlackroomStatusSummary } from '../../types'
import {
  getBlackroomBlockingReasonKey,
  resolveBlackroomGeoReadiness,
} from '../blackroom-status'

function buildStatus(
  overrides: Partial<BlackroomStatusSummary> = {}
): BlackroomStatusSummary {
  return {
    enabled: true,
    auto_ban_enabled: true,
    shadow_mode: false,
    realtime_enabled: true,
    geo_enabled: true,
    geo_effective: true,
    resolver: {
      ready: true,
      country_ready: true,
      asn_ready: true,
      version: 'country-1/asn-2',
    },
    blocking: [],
    ...overrides,
  }
}

describe('blackroom geo readiness', () => {
  test('effective when the resolver is ready', () => {
    assert.deepEqual(resolveBlackroomGeoReadiness(buildStatus()), {
      effective: true,
      reasonKey: null,
    })
  })

  test('reports the missing MMDB path when geo is on but nothing is configured', () => {
    const readiness = resolveBlackroomGeoReadiness(
      buildStatus({
        geo_effective: false,
        resolver: {
          ready: false,
          country_ready: false,
          asn_ready: false,
          version: '',
          error_code: 'not_configured',
        },
      })
    )

    assert.equal(readiness.effective, false)
    assert.equal(readiness.reasonKey, 'No MMDB path is configured.')
  })

  test('reports an unreadable MMDB file', () => {
    const readiness = resolveBlackroomGeoReadiness(
      buildStatus({
        geo_effective: false,
        resolver: {
          ready: false,
          country_ready: true,
          asn_ready: false,
          version: '',
          error_code: 'open_failed',
        },
      })
    )

    assert.equal(readiness.reasonKey, 'The MMDB file could not be opened.')
  })

  test('reports an uninitialized resolver', () => {
    const readiness = resolveBlackroomGeoReadiness(
      buildStatus({
        geo_effective: false,
        resolver: {
          ready: false,
          country_ready: false,
          asn_ready: false,
          version: '',
          error_code: 'not_initialized',
        },
      })
    )

    assert.equal(
      readiness.reasonKey,
      'The MMDB resolver has not been initialized.'
    )
  })

  test('falls back to the generic resolver reason for unknown error codes', () => {
    const readiness = resolveBlackroomGeoReadiness(
      buildStatus({
        geo_effective: false,
        resolver: {
          ready: false,
          country_ready: false,
          asn_ready: false,
          version: '',
          error_code: 'future_error',
        },
      })
    )

    assert.equal(readiness.reasonKey, 'The MMDB resolver is not ready.')
  })

  test('reports a disabled geo switch instead of a resolver problem', () => {
    const readiness = resolveBlackroomGeoReadiness(
      buildStatus({
        geo_enabled: false,
        geo_effective: false,
        resolver: {
          ready: false,
          country_ready: false,
          asn_ready: false,
          version: '',
        },
      })
    )

    assert.deepEqual(readiness, {
      effective: false,
      reasonKey: 'Geo blocking is disabled.',
    })
  })

  test('stays silent while the status payload is unavailable', () => {
    assert.deepEqual(resolveBlackroomGeoReadiness(undefined), {
      effective: false,
      reasonKey: null,
    })
  })
})

describe('blackroom blocking reason labels', () => {
  test('maps known reason codes to translation keys', () => {
    assert.equal(
      getBlackroomBlockingReasonKey('blackroom_disabled'),
      'Blackroom is disabled, so no ban is applied.'
    )
    assert.equal(
      getBlackroomBlockingReasonKey('auto_ban_disabled'),
      'Auto ban is disabled, so only manual bans apply.'
    )
    assert.equal(
      getBlackroomBlockingReasonKey('geo_resolver_not_ready'),
      'The MMDB resolver is not ready, so geo blocking is skipped.'
    )
  })

  test('leaves unknown reason codes for the caller to show verbatim', () => {
    assert.equal(getBlackroomBlockingReasonKey('future_reason'), null)
  })
})
