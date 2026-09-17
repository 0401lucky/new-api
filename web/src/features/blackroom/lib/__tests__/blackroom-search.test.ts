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

import { blackroomSearchSchema } from '../blackroom-search'

describe('blackroom URL search schema', () => {
  test('keeps absent IP audit parameters absent so the URL stays clean', () => {
    assert.deepEqual(blackroomSearchSchema.parse({}), {})
  })

  test('keeps valid IP audit parameters as typed values', () => {
    assert.deepEqual(
      blackroomSearchSchema.parse({
        tab: 'ip-audit',
        ipPage: 3,
        ipPageSize: 50,
        ipFilter: '203.0.113.7',
        ipStart: 1_754_800_000,
        ipEnd: 1_754_900_000,
      }),
      {
        tab: 'ip-audit',
        ipPage: 3,
        ipPageSize: 50,
        ipFilter: '203.0.113.7',
        ipStart: 1_754_800_000,
        ipEnd: 1_754_900_000,
      }
    )
  })

  test('falls back to defaults for hand-edited nonsense in the URL', () => {
    assert.deepEqual(
      blackroomSearchSchema.parse({
        ipPage: 'abc',
        ipPageSize: 'twenty',
        ipFilter: 42,
        ipStart: 'oops',
        ipEnd: null,
      }),
      { ipPage: 1, ipPageSize: 20, ipFilter: '', ipStart: 0, ipEnd: 0 }
    )
  })

  test('falls back for out-of-range numbers instead of trusting the URL', () => {
    assert.deepEqual(
      blackroomSearchSchema.parse({
        ipPage: 0,
        ipPageSize: 100_000,
        ipStart: -1,
        ipEnd: Number.POSITIVE_INFINITY,
      }),
      { ipPage: 1, ipPageSize: 20, ipStart: 0, ipEnd: 0 }
    )
  })

  test('keeps the ban record filters independent from the IP audit ones', () => {
    assert.deepEqual(
      blackroomSearchSchema.parse({
        page: '2',
        status: ['active'],
        source: ['nope'],
        tab: 'nope',
      }),
      { page: 1, status: ['active'], source: [], tab: 'bans' }
    )
  })
})
