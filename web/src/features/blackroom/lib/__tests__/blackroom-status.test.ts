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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { resolveBlackroomDisplayStatus } from '../../constants'

describe('blackroom display status', () => {
  const now = 1_754_900_000

  test('active ban past banned_until renders as expired', () => {
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'active', banned_until: now - 1 },
        now
      ),
      'expired'
    )
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'active', banned_until: now },
        now
      ),
      'expired'
    )
  })

  test('active ban before banned_until stays active', () => {
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'active', banned_until: now + 60 },
        now
      ),
      'active'
    )
  })

  test('permanent ban (banned_until = 0) stays active', () => {
    assert.equal(
      resolveBlackroomDisplayStatus({ status: 'active', banned_until: 0 }, now),
      'active'
    )
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'active', banned_until: null },
        now
      ),
      'active'
    )
  })

  test('terminal statuses are returned unchanged', () => {
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'released', banned_until: now - 1 },
        now
      ),
      'released'
    )
    assert.equal(
      resolveBlackroomDisplayStatus(
        { status: 'expired', banned_until: now - 1 },
        now
      ),
      'expired'
    )
  })
})
